// Package private contains the private evaluator suite for Scenario 4.
// The tests in this file are written against the seeded source: they
// reproduce the documented symptoms of the single planted condition and
// assert against the unseeded reference. They live in /private/ and are
// NOT shipped as part of the model-visible repository.
package private

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/money"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/pricing"
	"github.com/somagen/scenario4/internal/promotion"
	"github.com/somagen/scenario4/internal/storage"
)

// loadOverlapFixture reads the private overlap fixture.
func loadOverlapFixture(t *testing.T) (sku, wildcard domain.Promotion) {
	t.Helper()
	b, err := os.ReadFile("../testdata/fixtures/overlap.json")
	if err != nil {
		t.Fatalf("read overlap fixture: %v", err)
	}
	var f struct {
		SKURule      domain.Promotion `json:"sku_rule"`
		WildcardRule domain.Promotion `json:"wildcard_rule"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return f.SKURule, f.WildcardRule
}

// TestCondition_MAINT_DEF_01 reproduces the documented symptom of the
// planted condition: a promotion that qualifies through BOTH a
// SKU-specific rule and a wildcard channel rule is applied twice on
// the seeded source (duplicate amount, duplicate application id,
// duplicate audit entry).
//
// The test asserts the EXPECTED correct state (single application, 900
// yen). On the seeded source the assertion will fail because the
// resolver concatenates the two paths without an ID-based
// de-duplication step. On a fixed source the assertion will pass.
func TestCondition_MAINT_DEF_01(t *testing.T) {
	skuRule, wildcardRule := loadOverlapFixture(t)
	store := storage.NewMemoryStore()
	ctx := context.Background()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: skuRule.TenantID, Name: "T"})
	_ = store.UpsertProduct(ctx, domain.Product{
		ID: skuRule.TenantID + "/SKU-00001", TenantID: skuRule.TenantID, SKU: "SKU-00001", Name: "P", BaseYen: 1000,
	})
	// The private seeder registers the same logical promotion with
	// TWO rule rows: one SKU-specific, one channel-wildcard.
	if err := store.UpsertPromotion(ctx, skuRule); err != nil {
		t.Fatalf("upsert sku rule: %v", err)
	}
	if err := store.UpsertPromotion(ctx, wildcardRule); err != nil {
		t.Fatalf("upsert wildcard rule: %v", err)
	}

	logger := observability.Default("private")
	eng := pricing.NewEngine(store, logger, pricing.NewResultCache(8))

	d, err := eng.Quote(ctx, domain.PricingRequest{
		TenantID: skuRule.TenantID, CustomerID: "cust-001", SKU: "SKU-00001",
		Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}

	// Symptom 1: the same promotion id appears in the applied list
	// more than once because the resolver did not de-duplicate the
	// SKU-specific and wildcard paths.
	if len(d.AppliedIDs) != 1 {
		t.Fatalf("expected exactly 1 application visit, got %d: %v",
			len(d.AppliedIDs), d.AppliedIDs)
	}

	// Symptom 2: the audit list is "duplicated" — there is one audit
	// entry per application visit. The correct count is 1.
	if len(d.AuditIDs) != 1 {
		t.Fatalf("expected exactly 1 audit entry, got %d: %v",
			len(d.AuditIDs), d.AuditIDs)
	}

	// Symptom 3: the final amount reflects the duplicate discount
	// (10% on 1000 yen applied twice = 810 yen, not the clean
	// single-application 900 yen).
	if d.FinalYen != 900 {
		t.Fatalf("expected final amount 900 yen (single 10%% discount on 1000), got %d "+
			"(planted: 10%% discount applied twice)", d.FinalYen)
	}
}

// TestCondition_MAINT_DEF_01_ResolverDirect exercises the resolver
// directly to make the failure mode obvious in test output. It asserts
// the same expectations as TestCondition_MAINT_DEF_01 but bypasses the
// storage layer.
func TestCondition_MAINT_DEF_01_ResolverDirect(t *testing.T) {
	skuRule, wildcardRule := loadOverlapFixture(t)
	candidates := []domain.Promotion{skuRule, wildcardRule}
	applied, audit, _, final, _ := promotion.Apply(
		candidates,
		money.FromYen(1000),
		1,
		"SKU-00001",
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)
	if len(applied) != 1 {
		t.Fatalf("expected exactly 1 application, got %d: %v", len(applied), applied)
	}
	if len(audit) != 1 {
		t.Fatalf("expected exactly 1 audit id, got %d: %v", len(audit), audit)
	}
	if final.Yen() != 900 {
		t.Fatalf("expected 900 yen (single 10%% discount on 1000), got %d "+
			"(planted: discount applied twice)", final.Yen())
	}
}

func uniq(xs []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}
