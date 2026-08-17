// Package private — clean_baseline_test.go
//
// This file documents the behaviour of the CLEAN (unseeded) reference
// resolver. The reference keeps an ID-based de-duplication step; the
// visible seeded source omits it. The two files in this package form a
// pair: the seeded TestCondition_MAINT_DEF_01 in condition_test.go is
// expected to FAIL on the visible repo; this file describes what a
// correct fix would produce and is used as a documentation artefact for
// reviewers.
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

// applyCleanDeduplicated is a copy of the reference resolver with the
// missing ID-based de-duplication step restored. It exists only to
// document what a clean implementation would do.
func applyCleanDeduplicated(candidates []domain.Promotion, base money.Money, quantity int64, requestSKU string, at time.Time) (applied []string, audit []string, f money.Money) {
	f = base
	skuSpecific, wildcard := promotion.SelectByPath(candidates, requestSKU)
	ordered := append(append([]domain.Promotion(nil), skuSpecific...), wildcard...)
	seen := map[string]struct{}{}
	for _, p := range ordered {
		if _, ok := seen[p.ID]; ok {
			continue
		}
		seen[p.ID] = struct{}{}
		switch p.Type {
		case domain.PromotionPercent:
			f = f.MulPct(p.Value)
		case domain.PromotionAmount:
			f = f.Sub(money.FromFloatYen(p.Value))
		}
		f = money.RoundJPY(f)
		applied = append(applied, p.ID)
		audit = append(audit, "audit-clean-"+p.ID)
	}
	_ = quantity
	_ = at
	return
}

// TestCleanBaseline_DuplicateOnce proves that, with the missing
// de-duplication step restored, the same overlap fixture produces a
// single application and a final amount of 900 yen (one 10% discount on
// 1000). This is the reference behaviour for documentation purposes;
// it is not run against the seeded repo.
func TestCleanBaseline_DuplicateOnce(t *testing.T) {
	skuRule, wildcardRule := loadOverlapFixture(t)
	candidates := []domain.Promotion{skuRule, wildcardRule}
	applied, audit, final := applyCleanDeduplicated(
		candidates, money.FromYen(1000), 1, "SKU-00001",
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)
	if len(applied) != 1 {
		t.Fatalf("clean baseline expected 1 application, got %d: %v", len(applied), applied)
	}
	if len(audit) != 1 {
		t.Fatalf("clean baseline expected 1 audit id, got %d: %v", len(audit), audit)
	}
	if final.Yen() != 900 {
		t.Fatalf("clean baseline expected 900 yen, got %d", final.Yen())
	}
}

// TestCleanBaseline_EngineProducesSingleDiscount verifies the
// end-to-end engine with the missing dedup step: it would apply the
// promotion twice. This test is documentation-only; it is not part of
// the seeded-source verification.
func TestCleanBaseline_EngineProducesSingleDiscount(t *testing.T) {
	if os.Getenv("RUN_CLEAN_BASELINE") != "1" {
		t.Skip("clean baseline test is documentation-only; set RUN_CLEAN_BASELINE=1 to run")
	}
	skuRule, wildcardRule := loadOverlapFixture(t)
	store := storage.NewMemoryStore()
	ctx := context.Background()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: skuRule.TenantID, Name: "T"})
	_ = store.UpsertProduct(ctx, domain.Product{
		ID: skuRule.TenantID + "/SKU-00001", TenantID: skuRule.TenantID, SKU: "SKU-00001", Name: "P", BaseYen: 1000,
	})
	_ = store.UpsertPromotion(ctx, skuRule)
	_ = store.UpsertPromotion(ctx, wildcardRule)

	logger := observability.Default("clean")
	eng := pricing.NewEngine(store, logger, pricing.NewResultCache(8))
	d, err := eng.Quote(ctx, domain.PricingRequest{
		TenantID: skuRule.TenantID, CustomerID: "cust-001", SKU: "SKU-00001",
		Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if len(d.AppliedIDs) != 1 {
		t.Fatalf("expected exactly 1 application, got %d", len(d.AppliedIDs))
	}
}

// jsonMust is a small helper to keep the test files free of repeated
// error-handling boilerplate.
func jsonMust(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}
