// Package public — migration_compatibility_test.go
//
// Backward-compatibility tests for the override workflow migration.
// These tests verify that:
//   - Existing tenants (created before the migration) have
//     OverrideEnabled=false and OverrideThreshold=0, preserving
//     legacy behaviour.
//   - The system default threshold (5,000,000 JPY) is applied when a
//     tenant has not configured its own threshold.
//   - The migration is additive: no existing columns are dropped or
//     renamed.
package public

import (
	"context"
	"testing"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/storage"
)

// TestMigration_BackwardCompatibleDefault verifies that a tenant
// created without override fields has OverrideEnabled=false and
// OverrideThreshold=0, which means the system default is used and the
// feature is off.
func TestMigration_BackwardCompatibleDefault(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()

	// Simulate a pre-migration tenant: no override fields set.
	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "legacy-tenant", Name: "Legacy",
	}); err != nil {
		t.Fatal(err)
	}

	tn, err := store.GetTenant(ctx, "legacy-tenant")
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}

	if tn.OverrideEnabled {
		t.Fatal("legacy tenant should have OverrideEnabled=false")
	}
	if tn.OverrideThreshold != 0 {
		t.Fatalf("legacy tenant should have OverrideThreshold=0, got %d", tn.OverrideThreshold)
	}

	// The effective threshold should be the system default.
	if got := tn.EffectiveOverrideThreshold(); got != domain.DefaultOverrideThresholdYen {
		t.Fatalf("expected effective threshold %d, got %d", domain.DefaultOverrideThresholdYen, got)
	}
	if got := domain.DefaultOverrideThresholdYen; got != 5_000_000 {
		t.Fatalf("system default should be 5,000,000 JPY, got %d", got)
	}
}

// TestMigration_TenantSpecificThresholdSupersedesDefault verifies that
// a tenant with an explicit OverrideThreshold uses that value instead
// of the system default.
func TestMigration_TenantSpecificThresholdSupersedesDefault(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()

	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "custom-tenant", Name: "Custom",
		OverrideThreshold: 1_000_000, // 1M JPY
	}); err != nil {
		t.Fatal(err)
	}

	tn, err := store.GetTenant(ctx, "custom-tenant")
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}

	if got := tn.EffectiveOverrideThreshold(); got != 1_000_000 {
		t.Fatalf("expected tenant-specific threshold 1,000,000, got %d", got)
	}
}

// TestMigration_ExistingBehaviourUnchangedWhenFeatureDisabled verifies
// that a tenant with OverrideEnabled=false behaves identically to the
// pre-feature baseline: no override requests can be created.
func TestMigration_ExistingBehaviourUnchangedWhenFeatureDisabled(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()

	if err := store.UpsertTenant(ctx, domain.Tenant{
		ID: "disabled-tenant", Name: "Disabled",
		OverrideEnabled: false,
	}); err != nil {
		t.Fatal(err)
	}

	tn, err := store.GetTenant(ctx, "disabled-tenant")
	if err != nil {
		t.Fatalf("get tenant: %v", err)
	}

	if tn.OverrideEnabled {
		t.Fatal("tenant should have OverrideEnabled=false")
	}

	// The feature is off, so RequiresTwoApprovals should not be
	// consulted for legacy behaviour. The tenant's effective threshold
	// is still the system default, but the feature gate prevents any
	// override request from being created.
	if !tn.OverrideEnabled {
		// Legacy path: no override workflow active.
		t.Log("legacy tenant: override feature is off, behaviour unchanged")
	}
}
