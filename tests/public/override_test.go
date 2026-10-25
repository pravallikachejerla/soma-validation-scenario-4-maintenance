// Package public — override_test.go
//
// Public tests for the manual price-override approval workflow. These
// tests exercise the corrected requirements:
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
package public

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/somagen/scenario4/internal/application"
	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/storage"
)

func newOverrideService(t *testing.T) (*application.OverrideService, storage.Store, *observability.Metrics) {
	t.Helper()
	store := storage.NewMemoryStore()
	ctx := context.Background()
	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-a", Name: "Synthetic tenant-a",
		OverrideEnabled: true, OverrideThreshold: 3_000_000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-b", Name: "Synthetic tenant-b",
		OverrideEnabled: false, OverrideThreshold: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-c", Name: "Synthetic tenant-c",
		OverrideEnabled: true, OverrideThreshold: 0, // uses system default
	}); err != nil {
		t.Fatal(err)
	}
	logger := observability.Default("test")
	metrics := observability.NewMetrics()
	return application.NewOverrideService(store, logger), store, metrics
}

// TestOverride_DefaultThresholdIs5MillionYen verifies the system default
// is JPY 5,000,000 when a tenant has not configured its own threshold.
func TestOverride_DefaultThresholdIs5MillionYen(t *testing.T) {
	_, store, _ := newOverrideService(t)
	ctx := context.Background()
	tc, err := store.GetTenant(ctx, "tenant-c")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := tc.EffectiveOverrideThreshold(), int64(5_000_000); got != want {
		t.Fatalf("default threshold: got %d, want %d", got, want)
	}
	if !tc.RequiresTwoApprovals(5_000_000) {
		t.Fatalf("at-threshold override should require two approvals")
	}
	if tc.RequiresTwoApprovals(4_999_999) {
		t.Fatalf("below-threshold override should require one approval")
	}
}

// TestOverride_TenantSpecificThresholdSupersedesDefault verifies that a
// tenant's configured threshold takes precedence over the system
// default.
func TestOverride_TenantSpecificThresholdSupersedesDefault(t *testing.T) {
	_, store, _ := newOverrideService(t)
	ctx := context.Background()
	ta, err := store.GetTenant(ctx, "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ta.EffectiveOverrideThreshold(), int64(3_000_000); got != want {
		t.Fatalf("tenant-a threshold: got %d, want %d", got, want)
	}
	if !ta.RequiresTwoApprovals(3_000_000) {
		t.Fatalf("at-threshold override should require two approvals")
	}
	if ta.RequiresTwoApprovals(2_999_999) {
		t.Fatalf("below-threshold override should require one approval")
	}
}

// TestOverride_FeatureDisabledByDefault verifies that existing tenant
// behaviour is unchanged until the feature is explicitly enabled.
func TestOverride_FeatureDisabledByDefault(t *testing.T) {
	svc, _, _ := newOverrideService(t)
	ctx := context.Background()
	_, err := svc.CreateOverrideRequest(ctx,
		"tenant-b", "tenant-b-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test",
	)
	if !errors.Is(err, application.ErrOverrideFeatureDisabled) {
		t.Fatalf("expected ErrOverrideFeatureDisabled, got %v", err)
	}
}

// TestOverride_RequiresTwoApprovalsAtThreshold verifies that an override
// at or above the tenant's threshold requires two distinct approvals.
func TestOverride_RequiresTwoApprovalsAtThreshold(t *testing.T) {
	svc, _, _ := newOverrideService(t)
	ctx := context.Background()
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "tenant-a-requester", "SKU-00001", "web", "cust-001",
		1000, 3_000_000, "negotiated enterprise deal",
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if req.RequiredCount != 2 {
		t.Fatalf("expected 2 required approvals, got %d", req.RequiredCount)
	}
	// First approval: still pending.
	r1, err := svc.ApproveOverride(ctx, req.ID, "tenant-a-approver", "looks reasonable")
	if err != nil {
		t.Fatalf("first approve: %v", err)
	}
	if r1.Status != domain.OverrideStatusPending {
		t.Fatalf("expected pending after 1/2 approvals, got %s", r1.Status)
	}
	// Second approval: approved.
	r2, err := svc.ApproveOverride(ctx, req.ID, "tenant-a-admin", "second sign-off")
	if err != nil {
		t.Fatalf("second approve: %v", err)
	}
	if r2.Status != domain.OverrideStatusApproved {
		t.Fatalf("expected approved after 2/2 approvals, got %s", r2.Status)
	}
}

// TestOverride_OneApprovalBelowThreshold verifies that an override below
// the tenant's threshold requires only one approval.
func TestOverride_OneApprovalBelowThreshold(t *testing.T) {
	svc, _, _ := newOverrideService(t)
	ctx := context.Background()
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "tenant-a-requester", "SKU-00001", "web", "cust-001",
		1000, 2_999_999, "small adjustment",
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if req.RequiredCount != 1 {
		t.Fatalf("expected 1 required approval, got %d", req.RequiredCount)
	}
	r, err := svc.ApproveOverride(ctx, req.ID, "tenant-a-approver", "ok")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if r.Status != domain.OverrideStatusApproved {
		t.Fatalf("expected approved after 1/1, got %s", r.Status)
	}
}

// TestOverride_RequesterCannotApproveOwnRequest verifies the
// requester-vs-approver separation rule.
func TestOverride_RequesterCannotApproveOwnRequest(t *testing.T) {
	svc, _, _ := newOverrideService(t)
	ctx := context.Background()
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "tenant-a-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test",
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.ApproveOverride(ctx, req.ID, "tenant-a-requester", "self-approval attempt")
	if !errors.Is(err, application.ErrOverrideSelfApproval) {
		t.Fatalf("expected ErrOverrideSelfApproval, got %v", err)
	}
}

// TestOverride_ApprovalReasonMandatory verifies that an approval reason
// is required.
func TestOverride_ApprovalReasonMandatory(t *testing.T) {
	svc, _, _ := newOverrideService(t)
	ctx := context.Background()
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "tenant-a-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test",
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = svc.ApproveOverride(ctx, req.ID, "tenant-a-approver", "")
	if !errors.Is(err, application.ErrOverrideReasonMissing) {
		t.Fatalf("expected ErrOverrideReasonMissing, got %v", err)
	}
	_, err = svc.ApproveOverride(ctx, req.ID, "tenant-a-approver", "   ")
	if !errors.Is(err, application.ErrOverrideReasonMissing) {
		t.Fatalf("expected ErrOverrideReasonMissing for whitespace, got %v", err)
	}
}

// TestOverride_RejectionTerminatesRequest verifies that a single
// rejection terminates the request.
func TestOverride_RejectionTerminatesRequest(t *testing.T) {
	svc, _, _ := newOverrideService(t)
	ctx := context.Background()
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "tenant-a-requester", "SKU-00001", "web", "cust-001",
		1000, 3_000_000, "test",
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	r, err := svc.RejectOverride(ctx, req.ID, "tenant-a-approver", "price too low")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if r.Status != domain.OverrideStatusRejected {
		t.Fatalf("expected rejected, got %s", r.Status)
	}
	// Further decisions are rejected.
	_, err = svc.ApproveOverride(ctx, req.ID, "tenant-a-admin", "second sign-off")
	if !errors.Is(err, application.ErrOverrideAlreadyDecided) {
		t.Fatalf("expected ErrOverrideAlreadyDecided, got %v", err)
	}
}

// TestOverride_AuditCompleteness verifies that every request, approval,
// rejection and final pricing action is audited.
func TestOverride_AuditCompleteness(t *testing.T) {
	svc, store, _ := newOverrideService(t)
	ctx := context.Background()
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "tenant-a-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "audit test",
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.ApproveOverride(ctx, req.ID, "tenant-a-approver", "ok"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := svc.ApplyOverride(ctx, req.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}
	events, err := store.ListAudit(ctx, "tenant-a", 100)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	want := map[string]bool{
		"override.requested": false,
		"override.approved":  false,
		"override.applied":   false,
	}
	for _, e := range events {
		if _, ok := want[e.Action]; ok {
			want[e.Action] = true
		}
	}
	for action, seen := range want {
		if !seen {
			t.Fatalf("missing audit event %q (events=%d)", action, len(events))
		}
	}
}

// TestOverride_BackwardCompatibleDefault verifies that a tenant with no
// override configuration behaves as if the feature is disabled.
func TestOverride_BackwardCompatibleDefault(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	// Tenant with no override fields set at all.
	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-legacy", Name: "Legacy tenant",
	}); err != nil {
		t.Fatal(err)
	}
	logger := observability.Default("test")
	svc := application.NewOverrideService(store, logger)
	_, err := svc.CreateOverrideRequest(ctx,
		"tenant-legacy", "tenant-legacy-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test",
	)
	if !errors.Is(err, application.ErrOverrideFeatureDisabled) {
		t.Fatalf("expected ErrOverrideFeatureDisabled for legacy tenant, got %v", err)
	}
}

// TestOverride_ApplyRequiresApprovedState verifies that an override can
// only be applied after it has been approved.
func TestOverride_ApplyRequiresApprovedState(t *testing.T) {
	svc, _, _ := newOverrideService(t)
	ctx := context.Background()
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "tenant-a-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test",
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Try to apply before approval.
	_, err = svc.ApplyOverride(ctx, req.ID)
	if err == nil {
		t.Fatalf("expected error applying unapproved override")
	}
	// Approve then apply.
	if _, err := svc.ApproveOverride(ctx, req.ID, "tenant-a-approver", "ok"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	r, err := svc.ApplyOverride(ctx, req.ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if r.Status != domain.OverrideStatusApplied {
		t.Fatalf("expected applied, got %s", r.Status)
	}
	if r.AppliedAt == nil {
		t.Fatalf("expected applied_at to be set")
	}
}

// TestOverride_DuplicateApproverRejected verifies that the same approver
// cannot record two decisions on the same request.
func TestOverride_DuplicateApproverRejected(t *testing.T) {
	svc, _, _ := newOverrideService(t)
	ctx := context.Background()
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "tenant-a-requester", "SKU-00001", "web", "cust-001",
		1000, 3_000_000, "test",
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.ApproveOverride(ctx, req.ID, "tenant-a-approver", "first"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	_, err = svc.ApproveOverride(ctx, req.ID, "tenant-a-approver", "second")
	if err == nil {
		t.Fatalf("expected error on duplicate approver")
	}
}

// TestOverride_TimeFieldsPopulated verifies that created_at and
// updated_at are populated.
func TestOverride_TimeFieldsPopulated(t *testing.T) {
	svc, _, _ := newOverrideService(t)
	ctx := context.Background()
	before := time.Now().UTC().Add(-time.Second)
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "tenant-a-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test",
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)
	if req.CreatedAt.Before(before) || req.CreatedAt.After(after) {
		t.Fatalf("created_at out of range: %v", req.CreatedAt)
	}
	if req.UpdatedAt.Before(before) || req.UpdatedAt.After(after) {
		t.Fatalf("updated_at out of range: %v", req.UpdatedAt)
	}
}
