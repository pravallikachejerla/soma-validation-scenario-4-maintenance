// Package application — override_service_test.go
package application

import (
	"context"
	"testing"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/storage"
)

func newTestService() (*OverrideService, *storage.MemoryStore) {
	store := storage.NewMemoryStore()
	store.UpsertOverrideSettings(context.Background(), domain.TenantOverrideSettings{
		TenantID: "tenant-a", ThresholdYen: 1_000_000, RequiredApprovals: 2, OverrideEnabled: true,
	})
	return NewOverrideService(store), store
}

func TestOverride_TenantSpecificThreshold(t *testing.T) {
	svc, store := newTestService()
	store.UpsertOverrideSettings(context.Background(), domain.TenantOverrideSettings{
		TenantID: "tenant-b", ThresholdYen: 10_000_000, RequiredApprovals: 2, OverrideEnabled: true,
	})
	req, err := svc.CreateRequest("tenant-a", "user-1", "SKU-1", "web", 1000, 6_000_000, "test")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != domain.OverridePending {
		t.Fatalf("tenant-a: expected pending, got %s", req.Status)
	}
	req2, err := svc.CreateRequest("tenant-b", "user-1", "SKU-1", "web", 1000, 6_000_000, "test")
	if err != nil {
		t.Fatal(err)
	}
	if req2.Status != domain.OverrideApplied {
		t.Fatalf("tenant-b: expected applied (below threshold), got %s", req2.Status)
	}
}

func TestOverride_DefaultThresholdWhenUnset(t *testing.T) {
	store := storage.NewMemoryStore()
	store.UpsertOverrideSettings(context.Background(), domain.TenantOverrideSettings{
		TenantID: "tenant-new", OverrideEnabled: true,
	})
	svc := NewOverrideService(store)
	req, err := svc.CreateRequest("tenant-new", "user-1", "SKU-1", "web", 1000, 6_000_000, "test")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != domain.OverridePending {
		t.Fatalf("expected pending (above default 5M), got %s", req.Status)
	}
}

func TestOverride_BelowThreshold_NoApprovalRequired(t *testing.T) {
	svc, _ := newTestService()
	req, err := svc.CreateRequest("tenant-a", "user-1", "SKU-1", "web", 1000, 100_000, "test")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != domain.OverrideApplied {
		t.Fatalf("expected applied (below threshold), got %s", req.Status)
	}
}

func TestOverride_AboveThreshold_RequiresApproval(t *testing.T) {
	svc, _ := newTestService()
	req, err := svc.CreateRequest("tenant-a", "user-1", "SKU-1", "web", 1000, 6_000_000, "test")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != domain.OverridePending {
		t.Fatalf("expected pending, got %s", req.Status)
	}
}

func TestOverride_RequesterCannotSelfApprove(t *testing.T) {
	svc, _ := newTestService()
	req, _ := svc.CreateRequest("tenant-a", "user-1", "SKU-1", "web", 1000, 6_000_000, "test")
	_, err := svc.Approve(req.ID, "user-1", "self-approve attempt")
	if err != storage.ErrSelfApproval {
		t.Fatalf("expected ErrSelfApproval, got %v", err)
	}
}

func TestOverride_TwoDistinctApproversRequired(t *testing.T) {
	svc, _ := newTestService()
	req, _ := svc.CreateRequest("tenant-a", "user-1", "SKU-1", "web", 1000, 6_000_000, "test")
	r1, err := svc.Approve(req.ID, "user-2", "first approval")
	if err != nil {
		t.Fatal(err)
	}
	if r1.Status != domain.OverridePending {
		t.Fatalf("after 1 approval: expected pending, got %s", r1.Status)
	}
	_, err = svc.Approve(req.ID, "user-2", "duplicate")
	if err != storage.ErrDuplicateApprover {
		t.Fatalf("expected ErrDuplicateApprover, got %v", err)
	}
	r2, err := svc.Approve(req.ID, "user-3", "second approval")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Status != domain.OverrideApproved {
		t.Fatalf("after 2 distinct approvals: expected approved, got %s", r2.Status)
	}
}

func TestOverride_OneApprovalInsufficient(t *testing.T) {
	svc, _ := newTestService()
	req, _ := svc.CreateRequest("tenant-a", "user-1", "SKU-1", "web", 1000, 6_000_000, "test")
	r, err := svc.Approve(req.ID, "user-2", "only one")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != domain.OverridePending {
		t.Fatalf("after 1 approval: expected pending, got %s", r.Status)
	}
}

func TestOverride_ReasonRequired(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.CreateRequest("tenant-a", "user-1", "SKU-1", "web", 1000, 6_000_000, "")
	if err != ErrReasonRequired {
		t.Fatalf("expected ErrReasonRequired, got %v", err)
	}
}

func TestOverride_FeatureDisabledByDefault(t *testing.T) {
	store := storage.NewMemoryStore()
	svc := NewOverrideService(store)
	_, err := svc.CreateRequest("tenant-a", "user-1", "SKU-1", "web", 1000, 100, "test")
	if err != ErrFeatureDisabled {
		t.Fatalf("expected ErrFeatureDisabled, got %v", err)
	}
}

func TestOverride_AuditCompleteness(t *testing.T) {
	svc, store := newTestService()
	req, _ := svc.CreateRequest("tenant-a", "user-1", "SKU-1", "web", 1000, 6_000_000, "initial")
	svc.Approve(req.ID, "user-2", "first")
	svc.Approve(req.ID, "user-3", "second")
	audit, _ := store.ListOverrideAudit(context.Background(), req.ID)
	if len(audit) < 3 {
		t.Fatalf("expected at least 3 audit entries, got %d", len(audit))
	}
}
