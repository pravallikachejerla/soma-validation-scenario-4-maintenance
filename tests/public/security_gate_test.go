// Package public — security_gate_test.go
//
// Security gate tests for the override workflow. These tests verify
// that:
//   - A requester cannot approve their own request.
//   - An approval reason is mandatory.
//   - Duplicate approvals from the same approver are rejected.
//   - The override feature is gated by Tenant.OverrideEnabled.
//   - Audit events are recorded for every state transition.
package public

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/somagen/scenario4/internal/application"
	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/storage"
)

// TestSecurity_RequesterCannotApproveOwnRequest verifies that the
// requester of an override request cannot also be an approver.
func TestSecurity_RequesterCannotApproveOwnRequest(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	logger := observability.Default("test")

	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-a", Name: "T", OverrideEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	svc := application.NewOverrideService(store, logger)
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "user-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test reason")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// The requester tries to approve their own request.
	_, err = svc.ApproveOverride(ctx, req.ID, "user-requester", "self-approve attempt")
	if err == nil {
		t.Fatal("expected error when requester approves own request")
	}
	if !strings.Contains(err.Error(), "cannot approve their own request") {
		t.Fatalf("expected self-approval error, got: %v", err)
	}
}

// TestSecurity_ApprovalReasonMandatory verifies that an empty or
// whitespace-only reason is rejected.
func TestSecurity_ApprovalReasonMandatory(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	logger := observability.Default("test")

	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-a", Name: "T", OverrideEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	svc := application.NewOverrideService(store, logger)
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "user-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test reason")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Empty reason.
	_, err = svc.ApproveOverride(ctx, req.ID, "user-approver", "")
	if err == nil {
		t.Fatal("expected error for empty reason")
	}
	if !strings.Contains(err.Error(), "reason is required") {
		t.Fatalf("expected reason-required error, got: %v", err)
	}

	// Whitespace-only reason.
	_, err = svc.ApproveOverride(ctx, req.ID, "user-approver", "   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only reason")
	}
}

// TestSecurity_DuplicateApproverRejected verifies that the same
// approver cannot record two decisions on the same request.
func TestSecurity_DuplicateApproverRejected(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	logger := observability.Default("test")

	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-a", Name: "T", OverrideEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	svc := application.NewOverrideService(store, logger)
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "user-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test reason")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// First approval.
	_, err = svc.ApproveOverride(ctx, req.ID, "user-approver-1", "first approval")
	if err != nil {
		t.Fatalf("first approval: %v", err)
	}

	// Same approver tries again.
	_, err = svc.ApproveOverride(ctx, req.ID, "user-approver-1", "duplicate attempt")
	if err == nil {
		t.Fatal("expected error for duplicate approver")
	}
}

// TestSecurity_FeatureGatedByTenantFlag verifies that the override
// feature is off by default and must be explicitly enabled.
func TestSecurity_FeatureGatedByTenantFlag(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	logger := observability.Default("test")

	// Tenant with OverrideEnabled=false (default).
	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-a", Name: "T",
	}); err != nil {
		t.Fatal(err)
	}

	svc := application.NewOverrideService(store, logger)
	_, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "user-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test reason")
	if err == nil {
		t.Fatal("expected error when feature is disabled")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("expected feature-disabled error, got: %v", err)
	}
}

// TestSecurity_AuditCompleteness verifies that every state transition
// (request, approval, rejection, apply) produces an audit event.
func TestSecurity_AuditCompleteness(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	logger := observability.Default("test")

	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-a", Name: "T", OverrideEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	svc := application.NewOverrideService(store, logger)
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "user-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test reason")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Approve.
	_, err = svc.ApproveOverride(ctx, req.ID, "user-approver-1", "approved")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}

	// Apply.
	_, err = svc.ApplyOverride(ctx, req.ID)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Check audit events.
	events, err := store.ListAudit(ctx, "tenant-a", 100)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}

	actions := make(map[string]bool)
	for _, e := range events {
		actions[e.Action] = true
	}

	expected := []string{"override.requested", "override.approved", "override.applied"}
	for _, a := range expected {
		if !actions[a] {
			t.Fatalf("expected audit action %q, not found in %v", a, actions)
		}
	}
}

// TestSecurity_RejectionTerminatesRequest verifies that a single
// rejection moves the request to rejected state and prevents further
// approvals.
func TestSecurity_RejectionTerminatesRequest(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	logger := observability.Default("test")

	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-a", Name: "T", OverrideEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	svc := application.NewOverrideService(store, logger)
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "user-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test reason")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Reject.
	rejected, err := svc.RejectOverride(ctx, req.ID, "user-approver-1", "not justified")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if rejected.Status != domain.OverrideStatusRejected {
		t.Fatalf("expected rejected status, got %s", rejected.Status)
	}

	// Try to approve after rejection.
	_, err = svc.ApproveOverride(ctx, req.ID, "user-approver-2", "too late")
	if err == nil {
		t.Fatal("expected error when approving after rejection")
	}
}

// TestSecurity_TimeFieldsPopulated verifies that CreatedAt and
// UpdatedAt are set on every request.
func TestSecurity_TimeFieldsPopulated(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	logger := observability.Default("test")

	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-a", Name: "T", OverrideEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	svc := application.NewOverrideService(store, logger)
	before := time.Now().UTC()
	req, err := svc.CreateOverrideRequest(ctx,
		"tenant-a", "user-requester", "SKU-00001", "web", "cust-001",
		1000, 500, "test reason")
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if req.CreatedAt.Before(before) || req.CreatedAt.After(after) {
		t.Fatalf("CreatedAt %v not in [%v, %v]", req.CreatedAt, before, after)
	}
	if req.UpdatedAt.Before(before) || req.UpdatedAt.After(after) {
		t.Fatalf("UpdatedAt %v not in [%v, %v]", req.UpdatedAt, before, after)
	}
}
