// Package override implements the manual price-override approval workflow.
//
// A manual override lets an operator pin a JPY unit price for a
// specific (tenant, SKU, channel) triple for a bounded window, but only
// after it has cleared a two-distinct-approver review with role
// separation (requester ≠ approver, approver1 ≠ approver2). The
// override is gated by a per-tenant threshold: while disabled, the
// workflow is inactive; when enabled, only overrides whose OverrideYen
// is at or above the configured ThresholdYen may be requested.
//
// The package exposes a pure service (no I/O) that operates on a
// Repo. The service performs:
//
//   - request creation (state -> requested)
//   - first approval (state -> awaiting_second_approval) with the
//     constraint that the first approver is distinct from the requester
//   - second approval (state -> approved) with the constraint that the
//     second approver is distinct from both the requester and the first
//     approver
//   - rejection (any non-terminal state -> rejected)
//   - apply (approved -> applied; this is what activates the override)
//
// Every transition writes an audit entry and bumps the tenant config
// version so cached pricing decisions are invalidated.
package override

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/somagen/scenario4/internal/domain"
)

// Repo is the storage-side contract this package needs. The concrete
// implementation lives in internal/storage (memory + postgres) and
// adapts to the local types via the four methods below.
type Repo interface {
	CreateOverride(ctx context.Context, o domain.Override) error
	GetOverride(ctx context.Context, id string) (domain.Override, error)
	UpdateOverride(ctx context.Context, o domain.Override) error
	ListOverridesByTenant(ctx context.Context, tenantID string) ([]domain.Override, error)
	GetThreshold(ctx context.Context, tenantID string) (domain.OverrideThreshold, error)
	UpsertThreshold(ctx context.Context, t domain.OverrideThreshold) error
	AppendAudit(ctx context.Context, e domain.AuditEvent) error
	BumpConfigVersion(ctx context.Context, tenantID string) (int64, error)
}

// Sentinel errors returned by the service. Callers compare with
// errors.Is to decide UX (HTTP 4xx vs 5xx, retry semantics, etc).
var (
	// ErrOverrideNotEnabled is returned by Request when the per-tenant
	// threshold config is disabled. The workflow is inactive until an
	// operator explicitly enables it for the tenant.
	ErrOverrideNotEnabled = errors.New("override workflow not enabled for tenant")

	// ErrBelowThreshold is returned by Request when OverrideYen is
	// strictly below the configured per-tenant threshold. The threshold
	// is inclusive: OverrideYen >= threshold passes.
	ErrBelowThreshold = errors.New("override amount below tenant threshold")

	// ErrRoleSeparation is returned by FirstApprove / SecondApprove
	// when the approver violates role-separation (requester ==
	// approver, or first approver == second approver).
	ErrRoleSeparation = errors.New("role separation violated")

	// ErrInvalidStateTransition is returned when an operation is
	// requested from a state that does not allow it (e.g. FirstApprove
	// on an OverrideApplied override, Apply on a non-OverrideApproved
	// override).
	ErrInvalidStateTransition = errors.New("invalid state transition")

	// ErrNotApproved is returned by Apply when the override is not in
	// the OverrideApproved state. Apply only accepts OverrideApproved.
	ErrNotApproved = errors.New("override is not in approved state")

	// ErrAlreadyTerminal is returned by Reject when the override is in
	// a terminal state (OverrideRejected or OverrideApplied).
	ErrAlreadyTerminal = errors.New("override is already in a terminal state")
)

// RequestInput is the input to Request.
type RequestInput struct {
	TenantID    string
	SKU         string
	Channel     string
	Quantity    int64
	OverrideYen int64
	RequesterID string
	Reason      string
}

// Service is the override workflow. It is a pure function: all state
// is held in the underlying Repo; the Service itself only owns a clock
// for deterministic timestamps in tests.
type Service struct {
	Repo Repo
	Now  func() time.Time
}

// NewService wires an override Service.
func NewService(r Repo, clock func() time.Time) *Service {
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Service{Repo: r, Now: clock}
}

// Request creates a new override request. It validates the per-tenant
// threshold and refuses to create the override if the workflow is
// disabled for the tenant or the amount is below the threshold.
func (s *Service) Request(ctx context.Context, in RequestInput) (domain.Override, error) {
	if in.TenantID == "" || in.SKU == "" || in.Channel == "" {
		return domain.Override{}, errors.New("missing required field")
	}
	if in.RequesterID == "" {
		return domain.Override{}, errors.New("missing requester")
	}
	if in.OverrideYen <= 0 {
		return domain.Override{}, errors.New("override amount must be positive")
	}
	thr, err := s.Repo.GetThreshold(ctx, in.TenantID)
	if err != nil {
		return domain.Override{}, err
	}
	if !thr.Enabled {
		return domain.Override{}, ErrOverrideNotEnabled
	}
	if in.OverrideYen < thr.ThresholdYen {
		return domain.Override{}, ErrBelowThreshold
	}
	now := s.Now()
	o := domain.Override{
		ID:          "ovr-" + uuid.NewString(),
		TenantID:    in.TenantID,
		SKU:         in.SKU,
		Channel:     in.Channel,
		Quantity:    in.Quantity,
		OverrideYen: in.OverrideYen,
		RequesterID: in.RequesterID,
		State:       domain.OverrideRequested,
		Reason:      in.Reason,
		CreatedAt:   now,
		UpdatedAt:   now,
		// Default validity window: 24 hours from now.
		ValidFrom:  now,
		ValidUntil: now.Add(24 * time.Hour),
	}
	if err := s.Repo.CreateOverride(ctx, o); err != nil {
		return domain.Override{}, err
	}
	if err := s.appendAudit(ctx, o, "override.requested",
		"requester="+in.RequesterID+"; amount="+itoa(in.OverrideYen)+"; reason="+in.Reason,
	); err != nil {
		return domain.Override{}, err
	}
	return o, nil
}

// FirstApprove moves the override from requested -> awaiting_second_approval
// with the constraint that the approver is distinct from the requester.
func (s *Service) FirstApprove(ctx context.Context, id, approverID string) (domain.Override, error) {
	if approverID == "" {
		return domain.Override{}, errors.New("missing approver")
	}
	o, err := s.Repo.GetOverride(ctx, id)
	if err != nil {
		return domain.Override{}, err
	}
	if o.State != domain.OverrideRequested {
		return domain.Override{}, ErrInvalidStateTransition
	}
	if approverID == o.RequesterID {
		return domain.Override{}, ErrRoleSeparation
	}
	now := s.Now()
	o.Approver1ID = approverID
	o.State = domain.OverrideAwaitingSecondApproval
	o.UpdatedAt = now
	if err := s.Repo.UpdateOverride(ctx, o); err != nil {
		return domain.Override{}, err
	}
	if err := s.appendAudit(ctx, o, "override.first_approved",
		"approver1="+approverID,
	); err != nil {
		return domain.Override{}, err
	}
	return o, nil
}

// SecondApprove moves the override from awaiting_second_approval ->
// approved, with the constraint that the second approver is distinct
// from both the requester and the first approver.
func (s *Service) SecondApprove(ctx context.Context, id, approverID string) (domain.Override, error) {
	if approverID == "" {
		return domain.Override{}, errors.New("missing approver")
	}
	o, err := s.Repo.GetOverride(ctx, id)
	if err != nil {
		return domain.Override{}, err
	}
	if o.State != domain.OverrideAwaitingSecondApproval {
		return domain.Override{}, ErrInvalidStateTransition
	}
	if approverID == o.RequesterID || approverID == o.Approver1ID {
		return domain.Override{}, ErrRoleSeparation
	}
	now := s.Now()
	o.Approver2ID = approverID
	o.State = domain.OverrideApproved
	o.UpdatedAt = now
	if err := s.Repo.UpdateOverride(ctx, o); err != nil {
		return domain.Override{}, err
	}
	if err := s.appendAudit(ctx, o, "override.second_approved",
		"approver2="+approverID,
	); err != nil {
		return domain.Override{}, err
	}
	return o, nil
}

// Reject moves the override from any non-terminal state to rejected.
// Rejection of an already-terminal override returns ErrAlreadyTerminal.
func (s *Service) Reject(ctx context.Context, id, approverID, reason string) (domain.Override, error) {
	if approverID == "" {
		return domain.Override{}, errors.New("missing approver")
	}
	o, err := s.Repo.GetOverride(ctx, id)
	if err != nil {
		return domain.Override{}, err
	}
	if o.State == domain.OverrideRejected || o.State == domain.OverrideApplied {
		return domain.Override{}, ErrAlreadyTerminal
	}
	now := s.Now()
	o.State = domain.OverrideRejected
	o.RejectReason = reason
	o.UpdatedAt = now
	if err := s.Repo.UpdateOverride(ctx, o); err != nil {
		return domain.Override{}, err
	}
	if err := s.appendAudit(ctx, o, "override.rejected",
		"approver="+approverID+"; reason="+reason,
	); err != nil {
		return domain.Override{}, err
	}
	return o, nil
}

// Apply moves the override from approved -> applied, sets AppliedAt,
// and bumps the tenant config version so cached pricing decisions are
// invalidated. Apply only accepts OverrideApproved.
func (s *Service) Apply(ctx context.Context, id, actorID string) (domain.Override, error) {
	if actorID == "" {
		return domain.Override{}, errors.New("missing actor")
	}
	o, err := s.Repo.GetOverride(ctx, id)
	if err != nil {
		return domain.Override{}, err
	}
	if o.State != domain.OverrideApproved {
		return domain.Override{}, ErrNotApproved
	}
	now := s.Now()
	o.State = domain.OverrideApplied
	o.AppliedAt = now
	o.UpdatedAt = now
	if err := s.Repo.UpdateOverride(ctx, o); err != nil {
		return domain.Override{}, err
	}
	if _, err := s.Repo.BumpConfigVersion(ctx, o.TenantID); err != nil {
		return domain.Override{}, err
	}
	if err := s.appendAudit(ctx, o, "override.applied",
		"actor="+actorID+"; applied_at="+now.Format(time.RFC3339),
	); err != nil {
		return domain.Override{}, err
	}
	return o, nil
}

// EnableForTenant enables the override workflow for a tenant with the
// given threshold. The default threshold is the product spec default
// (JPY 5,000,000) if thresholdYen is 0.
func (s *Service) EnableForTenant(ctx context.Context, tenantID string, thresholdYen int64, actorID string) (domain.OverrideThreshold, error) {
	if thresholdYen == 0 {
		thresholdYen = domain.DefaultOverrideThresholdYen
	}
	thr := domain.OverrideThreshold{
		TenantID:     tenantID,
		Enabled:      true,
		ThresholdYen: thresholdYen,
		UpdatedAt:    s.Now(),
		UpdatedBy:    actorID,
	}
	if err := s.Repo.UpsertThreshold(ctx, thr); err != nil {
		return domain.OverrideThreshold{}, err
	}
	_ = s.Repo.AppendAudit(ctx, domain.AuditEvent{
		ID: "audit-" + uuid.NewString(), TenantID: tenantID,
		Action:  "override.threshold_enabled",
		Subject: tenantID,
		Detail:  "threshold_yen=" + itoa(thresholdYen) + "; actor=" + actorID,
	})
	return thr, nil
}

// appendAudit is the package's single audit-write path.
func (s *Service) appendAudit(ctx context.Context, o domain.Override, action, detail string) error {
	return s.Repo.AppendAudit(ctx, domain.AuditEvent{
		ID:        "audit-" + uuid.NewString(),
		TenantID:  o.TenantID,
		Action:    action,
		Subject:   o.ID,
		Detail:    detail,
		CreatedAt: s.Now(),
	})
}

// itoa is a tiny int64-to-string helper to avoid pulling in strconv
// for the audit detail strings.
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
