// Package application — override_service.go
//
// OverrideService enforces the manual price-override approval workflow:
// per-tenant threshold, two approvals required, requester cannot
// self-approve, mandatory reason, full audit trail.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/storage"
)

var (
	ErrFeatureDisabled = errors.New("override feature is disabled for this tenant")
	ErrBelowThreshold  = errors.New("override is below threshold; no approval required")
	ErrAlreadyDecided  = errors.New("override request has already been decided")
	ErrReasonRequired  = errors.New("reason is required")
)

type OverrideService struct {
	repo storage.OverrideRepo
	now  func() time.Time
}

func NewOverrideService(repo storage.OverrideRepo) *OverrideService {
	return &OverrideService{repo: repo, now: time.Now}
}

// CreateRequest creates a new override request. If the override is
// below the tenant's threshold, it is applied immediately without
// requiring approval.
func (s *OverrideService) CreateRequest(tenantID, requesterID, sku, channel string, basePriceYen, overrideYen int64, reason string) (domain.OverrideRequest, error) {
	if reason == "" {
		return domain.OverrideRequest{}, ErrReasonRequired
	}
	ctx := context.Background()
	settings, err := s.repo.GetOverrideSettings(ctx, tenantID)
	if err != nil {
		return domain.OverrideRequest{}, err
	}
	if !settings.OverrideEnabled {
		return domain.OverrideRequest{}, ErrFeatureDisabled
	}

	now := s.now()
	req := domain.OverrideRequest{
		ID:           "ovr-" + uuid.NewString(),
		TenantID:     tenantID,
		RequesterID:  requesterID,
		SKU:          sku,
		Channel:      channel,
		BasePriceYen: basePriceYen,
		OverrideYen:  overrideYen,
		Reason:       reason,
		Status:       domain.OverridePending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// If override is below threshold, apply immediately.
	if overrideYen < settings.ThresholdYen {
		req.Status = domain.OverrideApplied
	}

	if err := s.repo.CreateOverrideRequest(ctx, req); err != nil {
		return domain.OverrideRequest{}, err
	}
	_ = s.repo.AppendOverrideAudit(ctx, req.ID, "request_created", requesterID, reason)
	if req.Status == domain.OverrideApplied {
		_ = s.repo.AppendOverrideAudit(ctx, req.ID, "applied", requesterID, "below threshold")
	}
	return req, nil
}

// Approve records an approval. The requester cannot approve their own
// request. Each approver can only approve once.
func (s *OverrideService) Approve(requestID, approverID, reason string) (domain.OverrideRequest, error) {
	if reason == "" {
		return domain.OverrideRequest{}, ErrReasonRequired
	}
	ctx := context.Background()
	req, err := s.repo.GetOverrideRequest(ctx, requestID)
	if err != nil {
		return domain.OverrideRequest{}, err
	}
	if req.Status != domain.OverridePending {
		return domain.OverrideRequest{}, ErrAlreadyDecided
	}
	if req.RequesterID == approverID {
		return domain.OverrideRequest{}, storage.ErrSelfApproval
	}
	for _, a := range req.Approvals {
		if a.ApproverID == approverID {
			return domain.OverrideRequest{}, storage.ErrDuplicateApprover
		}
	}

	settings, err := s.repo.GetOverrideSettings(ctx, req.TenantID)
	if err != nil {
		return domain.OverrideRequest{}, err
	}

	req.Approvals = append(req.Approvals, domain.Approval{
		ApproverID: approverID,
		Decision:   "approved",
		Reason:     reason,
		At:         s.now(),
	})
	req.UpdatedAt = s.now()

	approvedCount := 0
	for _, a := range req.Approvals {
		if a.Decision == "approved" {
			approvedCount++
		}
	}
	if approvedCount >= settings.RequiredApprovals {
		req.Status = domain.OverrideApproved
	}

	if err := s.repo.UpdateOverrideRequest(ctx, req); err != nil {
		return domain.OverrideRequest{}, err
	}
	_ = s.repo.AppendOverrideAudit(ctx, req.ID, "approved", approverID, reason)
	if req.Status == domain.OverrideApproved {
		_ = s.repo.AppendOverrideAudit(ctx, req.ID, "approved_final", approverID, "threshold met")
	}
	return req, nil
}

// Reject records a rejection. Any approver can reject; rejection is
// final.
func (s *OverrideService) Reject(requestID, approverID, reason string) (domain.OverrideRequest, error) {
	if reason == "" {
		return domain.OverrideRequest{}, ErrReasonRequired
	}
	ctx := context.Background()
	req, err := s.repo.GetOverrideRequest(ctx, requestID)
	if err != nil {
		return domain.OverrideRequest{}, err
	}
	if req.Status != domain.OverridePending {
		return domain.OverrideRequest{}, ErrAlreadyDecided
	}
	if req.RequesterID == approverID {
		return domain.OverrideRequest{}, storage.ErrSelfApproval
	}

	req.Approvals = append(req.Approvals, domain.Approval{
		ApproverID: approverID,
		Decision:   "rejected",
		Reason:     reason,
		At:         s.now(),
	})
	req.Status = domain.OverrideRejected
	req.UpdatedAt = s.now()

	if err := s.repo.UpdateOverrideRequest(ctx, req); err != nil {
		return domain.OverrideRequest{}, err
	}
	_ = s.repo.AppendOverrideAudit(ctx, req.ID, "rejected", approverID, reason)
	return req, nil
}

// GetRequest returns the current state of an override request.
func (s *OverrideService) GetRequest(requestID string) (domain.OverrideRequest, error) {
	return s.repo.GetOverrideRequest(context.Background(), requestID)
}

// ListRequests returns all override requests for a tenant.
func (s *OverrideService) ListRequests(tenantID string) ([]domain.OverrideRequest, error) {
	return s.repo.ListOverrideRequests(context.Background(), tenantID)
}
