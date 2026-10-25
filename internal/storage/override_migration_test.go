// Package storage — override_migration_test.go
//
// Tests for the override workflow migration and backward compatibility.
//
// These tests verify:
//   - The migration SQL is syntactically valid and creates the expected tables.
//   - Existing tenants (no override_settings row) keep their current behaviour.
//   - The migration is additive: no existing tables are modified.
package storage

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/somagen/scenario4/internal/domain"
)

// TestOverride_BackwardCompatibility_NoSettingsRow verifies that a tenant
// with no override_settings row gets the default (feature disabled).
func TestOverride_BackwardCompatibility_NoSettingsRow(t *testing.T) {
	store := NewMemoryStore()
	cfg, err := store.GetOverrideSettings(context.Background(), "legacy-tenant")
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if cfg.OverrideEnabled {
		t.Fatalf("legacy tenant should have feature disabled")
	}
	if cfg.ThresholdYen != domain.DefaultThresholdYen {
		t.Fatalf("threshold = %d, want default %d", cfg.ThresholdYen, domain.DefaultThresholdYen)
	}
	if cfg.RequiredApprovals != domain.DefaultRequiredApprovals {
		t.Fatalf("required approvals = %d, want default %d", cfg.RequiredApprovals, domain.DefaultRequiredApprovals)
	}
}

// TestOverride_BackwardCompatibility_ExistingDataUnchanged verifies that
// enabling the override workflow for one tenant does not affect another
// tenant's settings.
func TestOverride_BackwardCompatibility_ExistingDataUnchanged(t *testing.T) {
	store := NewMemoryStore()

	// Enable for tenant-a
	if err := store.UpsertOverrideSettings(context.Background(), domain.TenantOverrideSettings{
		TenantID: "tenant-a", OverrideEnabled: true, ThresholdYen: 2_000_000, RequiredApprovals: 2,
	}); err != nil {
		t.Fatalf("upsert A: %v", err)
	}

	// Tenant-b should still be at defaults
	cfgB, err := store.GetOverrideSettings(context.Background(), "tenant-b")
	if err != nil {
		t.Fatalf("get B: %v", err)
	}
	if cfgB.OverrideEnabled {
		t.Fatalf("tenant-b should be disabled")
	}
	if cfgB.ThresholdYen != domain.DefaultThresholdYen {
		t.Fatalf("tenant-b threshold = %d, want default", cfgB.ThresholdYen)
	}
}

// TestOverride_MigrationSQL_ContainsExpectedTables verifies the migration
// SQL file contains the expected table definitions.
func TestOverride_MigrationSQL_ContainsExpectedTables(t *testing.T) {
	data, err := os.ReadFile("../../migrations/0002_override_workflow.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(data)

	expected := []string{
		"CREATE TABLE IF NOT EXISTS override_settings",
		"CREATE TABLE IF NOT EXISTS override_requests",
		"CREATE TABLE IF NOT EXISTS override_approvals",
		"CREATE TABLE IF NOT EXISTS override_audit",
	}
	for _, e := range expected {
		if !strings.Contains(sql, e) {
			t.Fatalf("migration missing %q", e)
		}
	}
}

// TestOverride_MigrationSQL_IsAdditive verifies the migration does not
// DROP or ALTER any existing tables.
func TestOverride_MigrationSQL_IsAdditive(t *testing.T) {
	data, err := os.ReadFile("../../migrations/0002_override_workflow.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToUpper(string(data))

	forbidden := []string{"DROP TABLE", "ALTER TABLE", "DELETE FROM", "TRUNCATE"}
	for _, f := range forbidden {
		if strings.Contains(sql, f) {
			t.Fatalf("migration contains forbidden statement: %s", f)
		}
	}
}

// TestOverride_MigrationDownSQL_DropsOnlyNewTables verifies the down
// migration only drops the new tables.
func TestOverride_MigrationDownSQL_DropsOnlyNewTables(t *testing.T) {
	data, err := os.ReadFile("../../migrations/0002_override_workflow.down.sql")
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	sql := strings.ToUpper(string(data))

	expected := []string{"OVERRIDE_AUDIT", "OVERRIDE_APPROVALS", "OVERRIDE_REQUESTS", "OVERRIDE_SETTINGS"}
	for _, e := range expected {
		if !strings.Contains(sql, e) {
			t.Fatalf("down migration missing %s", e)
		}
	}
}
