// Package application — override_service.go
//
// OverrideService implements the manual price-override approval workflow
// described in the corrected requirements:
//
//   - The override threshold is configurable per tenant, with a default
//     of JPY 5,000,000 applied when a tenant has no explicit value set.
//   - Existing tenant behaviour must remain unchanged until the feature
//     is enabled (Tenant.OverrideEnabled == false).
//   - Overrides at or above the threshold require two distinct approvals.
//   - A requester cannot approve their own request.
//   - An approval reason is mandatory.
//   - Every request, approval, rejection and final pricing action is
//     audited.
//
// The service is intentionally additive: it does not change the public
// pricing API. The pricing engine continues to call promotion.Apply
// unchanged; the override workflow is a separate, opt-in path.
package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/storage"
)

// OverrideService is the application-level service for manual price
// overrides.
type OverrideService struct {
	Store  storage.Store
	Logger *observability.Logger
}

// NewOverrideService wires an OverrideService.
func NewOverrideService(s storage.Store, l *observability.Logger) *OverrideService {
	return &OverrideService{Store: s, Logger: l}
}

// Sentinel errors returned by the override service. Handlers translate
// these into HTTP status codes.
var (
	ErrOverrideFeatureDisabled = errors.New("override feature is not enabled for this tenant")
	ErrOverrideThresholdZero   = errors.New("override amount must be positive")
	ErrOverrideReasonMissing   = errors.New("override reason is required")
	ErrOverrideSelfApproval    = errors.New("requester cannot approve their own request")
	ErrOverrideAlreadyDecided  = errors.New("override request is already decided")
	ErrOverrideNotFound        = errors.New("override request not found")
	ErrOverrideApproverMissing = errors.New("approver id is required")
)

// CreateOverrideRequest registers a new override request. The required
// approval count is computed from the tenant's effective threshold:
// 1 below threshold, 2 at-or-above threshold. The request is recorded
// in pending state and an audit event is appended.
func (o *OverrideService) CreateOverrideRequest(
	ctx context.Context,
	tenantID, requesterID, sku, channel, customerID string,
	originalYen, overrideYen int64,
	reason string,
) (domain.OverrideRequest, error) {
	tenant, err := o.Store.GetTenant(ctx, tenantID)
	if err != nil {
		return domain.OverrideRequest{}, fmt.Errorf("lookup tenant: %w", err)
	}
	if !tenant.OverrideEnabled {
		return domain.OverrideRequest{}, ErrOverrideFeatureDisabled
	}
	if overrideYen <= 0 {
		return domain.OverrideRequest{}, ErrOverrideThresholdZero
	}
	if strings.TrimSpace(reason) == "" {
		return domain.OverrideRequest{}, ErrOverrideReasonMissing
	}
	required := 1
	if tenant.RequiresTwoApprovals(overrideYen) {
		required = 2
	}
	now := time.Now().UTC()
	req := domain.OverrideRequest{
		ID:            "ovr-" + uuid.NewString(),
		TenantID:      tenantID,
		RequesterID:   requesterID,
		SKU:           sku,
		Channel:       channel,
		CustomerID:    customerID,
		OriginalYen:   originalYen,
		OverrideYen:   overrideYen,
		Reason:        reason,
		Status:        domain.OverrideStatusPending,
		Approvals:     []domain.OverrideApproval{},
		RequiredCount: required,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := o.Store.CreateOverrideRequest(ctx, req); err != nil {
		return domain.OverrideRequest{}, err
	}
	o.audit(ctx, tenantID, "override.requested", req.ID, fmt.Sprintf(
		"requester=%s; sku=%s; channel=%s; original=%d; override=%d; required=%d; reason=%s",
		requesterID, sku, channel, originalYen, overrideYen, required, reason,
	))
	return req, nil
}

// ApproveOverride records an approval against an override request. The
// approver must differ from the requester and must supply a non-empty
// reason. When the request has collected the required number of
// approvals, its status transitions to approved.
func (o *OverrideService) ApproveOverride(
	ctx context.Context,
	requestID, approverID, reason string,
) (domain.OverrideRequest, error) {
	if strings.TrimSpace(approverID) == "" {
		return domain.OverrideRequest{}, ErrOverrideApproverMissing
	}
	if strings.TrimSpace(reason) == "" {
		return domain.OverrideRequest{}, ErrOverrideReasonMissing
	}
	req, err := o.Store.GetOverrideRequest(ctx, requestID)
	if err != nil {
		return domain.OverrideRequest{}, ErrOverrideNotFound
	}
	if req.Status != domain.OverrideStatusPending {
		return domain.OverrideRequest{}, ErrOverrideAlreadyDecided
	}
	if approverID == req.RequesterID {
		return domain.OverrideRequest{}, ErrOverrideSelfApproval
	}
	// Reject if any prior approval was a rejection.
	for _, a := range req.Approvals {
		if a.Decision == "reject" {
			return domain.OverrideRequest{}, ErrOverrideAlreadyDecided
		}
	}
	// Reject duplicate approvals from the same approver.
	for _, a := range req.Approvals {
		if a.ApproverID == approverID {
			return domain.OverrideRequest{}, fmt.Errorf("approver %s already recorded a decision", approverID)
		}
	}
	req.Approvals = append(req.Approvals, domain.OverrideApproval{
		ApproverID: approverID,
		Decision:   "approve",
		Reason:     reason,
		At:         time.Now().UTC(),
	})
	req.UpdatedAt = time.Now().UTC()
	approveCount := 0
	for _, a := range req.Approvals {
		if a.Decision == "approve" {
			approveCount++
		}
	}
	if approveCount >= req.RequiredCount {
		req.Status = domain.OverrideStatusApproved
	}
	if err := o.Store.UpdateOverrideRequest(ctx, req); err != nil {
		return domain.OverrideRequest{}, err
	}
	o.audit(ctx, req.TenantID, "override.approved", req.ID, fmt.Sprintf(
		"approver=%s; approvals=%d/%d; reason=%s",
		approverID, approveCount, req.RequiredCount, reason,
	))
	return req, nil
}

// RejectOverride records a rejection. A single rejection terminates the
// request.
func (o *OverrideService) RejectOverride(
	ctx context.Context,
	requestID, approverID, reason string,
) (domain.OverrideRequest, error) {
	if strings.TrimSpace(approverID) == "" {
		return domain.OverrideRequest{}, ErrOverrideApproverMissing
	}
	if strings.TrimSpace(reason) == "" {
		return domain.OverrideRequest{}, ErrOverrideReasonMissing
	}
	req, err := o.Store.GetOverrideRequest(ctx, requestID)
	if err != nil {
		return domain.OverrideRequest{}, ErrOverrideNotFound
	}
	if req.Status != domain.OverrideStatusPending {
		return domain.OverrideRequest{}, ErrOverrideAlreadyDecided
	}
	if approverID == req.RequesterID {
		return domain.OverrideRequest{}, ErrOverrideSelfApproval
	}
	req.Approvals = append(req.Approvals, domain.OverrideApproval{
		ApproverID: approverID,
		Decision:   "reject",
		Reason:     reason,
		At:         time.Now().UTC(),
	})
	req.Status = domain.OverrideStatusRejected
	req.UpdatedAt = time.Now().UTC()
	if err := o.Store.UpdateOverrideRequest(ctx, req); err != nil {
		return domain.OverrideRequest{}, err
	}
	o.audit(ctx, req.TenantID, "override.rejected", req.ID, fmt.Sprintf(
		"approver=%s; reason=%s", approverID, reason,
	))
	return req, nil
}

// ApplyOverride marks an approved override as applied and writes the
// final pricing decision. The override is recorded as a synthetic
// promotion so the audit trail captures the final price.
func (o *OverrideService) ApplyOverride(
	ctx context.Context,
	requestID string,
) (domain.OverrideRequest, error) {
	req, err := o.Store.GetOverrideRequest(ctx, requestID)
	if err != nil {
		return domain.OverrideRequest{}, ErrOverrideNotFound
	}
	if req.Status != domain.OverrideStatusApproved {
		return domain.OverrideRequest{}, fmt.Errorf("override is not in approved state (current=%s)", req.Status)
	}
	now := time.Now().UTC()
	req.Status = domain.OverrideStatusApplied
	req.AppliedAt = &now
	req.UpdatedAt = now
	if err := o.Store.UpdateOverrideRequest(ctx, req); err != nil {
		return domain.OverrideRequest{}, err
	}
	o.audit(ctx, req.TenantID, "override.applied", req.ID, fmt.Sprintf(
		"final_yen=%d; original_yen=%d; sku=%s; channel=%s",
		req.OverrideYen, req.OriginalYen, req.SKU, req.Channel,
	))
	return req, nil
}

// ListOverrides returns override requests for a tenant, newest first.
func (o *OverrideService) ListOverrides(ctx context.Context, tenantID string, limit int) ([]domain.OverrideRequest, error) {
	out, err := o.Store.ListOverrideRequests(ctx, tenantID, limit)
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// GetOverride returns a single override request by id.
func (o *OverrideService) GetOverride(ctx context.Context, id string) (domain.OverrideRequest, error) {
	return o.Store.GetOverrideRequest(ctx, id)
}

func (o *OverrideService) audit(ctx context.Context, tenantID, action, subject, detail string) {
	if o.Store == nil {
		return
	}
	_ = o.Store.AppendAudit(ctx, domain.AuditEvent{
		ID:       "audit-" + uuid.NewString(),
		TenantID: tenantID,
		Action:   action,
		Subject:  subject,
		Detail:   detail,
	})
}
