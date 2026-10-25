// Package public — defect_reproduction_test.go
//
// This file contains the FAILING reproduction test for the
// duplicate-discount defect, written BEFORE the fix was applied. It
// pins the exact shape of the bug so that any regression to the
// resolver's de-duplication logic will be caught immediately.
//
// The test seeds the store with a single promotion id that has TWO
// rule rows: one SKU-specific and one channel-wildcard. Before the
// fix, the resolver would apply the promotion twice (once per path),
// producing a final price of 810 (1000 * 0.9 * 0.9) instead of the
// correct 900 (1000 * 0.9).
//
// This test is the TDD anchor for the defect correction.
package public

import (
	"context"
	"testing"
	"time"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/pricing"
	"github.com/somagen/scenario4/internal/storage"
)

// TestDefectReproduction_DuplicateDiscountViaOverlap is the failing
// reproduction test for the duplicate-discount defect. It seeds the
// store with a single promotion id that has two rule rows (SKU-specific
// and channel-wildcard) and asserts that the promotion is applied
// exactly once. Before the fix, this test would fail because the
// resolver would apply the promotion twice.
func TestDefectReproduction_DuplicateDiscountViaOverlap(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()

	// Seed tenant and product.
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "T"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertProduct(ctx, domain.Product{
		ID: "tenant-a/SKU-00001", TenantID: "tenant-a", SKU: "SKU-00001",
		Name: "P", BaseYen: 1000,
	}); err != nil {
		t.Fatal(err)
	}

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	// The defect: same promotion id, two rule rows.
	// Row 1: SKU-specific (matches the request's SKU).
	if err := store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-defect", TenantID: "tenant-a", Name: "Defect SKU",
		Type: domain.PromotionPercent, Value: 10.0, Channel: "web",
		SKU: "SKU-00001", Priority: 5, ValidFrom: from, ValidUntil: until,
	}); err != nil {
		t.Fatal(err)
	}
	// Row 2: channel-wildcard (matches the request's channel).
	if err := store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-defect", TenantID: "tenant-a", Name: "Defect wildcard",
		Type: domain.PromotionPercent, Value: 10.0, Channel: "web",
		SKU: "", Priority: 5, ValidFrom: from, ValidUntil: until,
	}); err != nil {
		t.Fatal(err)
	}

	logger := observability.Default("test")
	eng := pricing.NewEngine(store, logger, pricing.NewResultCache(64))

	d, err := eng.Quote(ctx, domain.PricingRequest{
		TenantID: "tenant-a", CustomerID: "cust-001", SKU: "SKU-00001",
		Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}

	// The defect would produce len(AppliedIDs) == 2 and FinalYen == 810.
	// The fix produces len(AppliedIDs) == 1 and FinalYen == 900.
	if len(d.AppliedIDs) != 1 {
		t.Fatalf("DEFECT REPRODUCED: expected 1 applied id, got %d (%v) — the promotion was applied more than once",
			len(d.AppliedIDs), d.AppliedIDs)
	}
	if d.FinalYen != 900 {
		t.Fatalf("DEFECT REPRODUCED: expected 900 (1000 - 10%%), got %d — the discount was applied twice", d.FinalYen)
	}
}
