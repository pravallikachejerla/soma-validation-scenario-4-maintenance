// Package public — duplicate_discount_test.go
//
// Public regression tests for the duplicate-discount defect. The defect
// was: when a promotion qualifies through both an SKU-specific path and
// a channel-wildcard path, the same discount was applied twice. The
// fix is in internal/promotion/resolver.go: a `seen` map de-duplicates
// by promotion id so each promotion is applied at most once.
//
// These tests cover the interactive and batch pricing paths, the
// overlapping-rule case, the wildcard case, and the repeated-path
// case.
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

// seedOverlapPromotion registers the same promotion id with two rule
// rows: one SKU-specific and one channel-wildcard. This is the exact
// shape that triggered the original defect.
func seedOverlapPromotion(t *testing.T, store storage.Store, tenantID, sku string, baseYen int64, pct float64) {
	t.Helper()
	ctx := context.Background()
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: tenantID, Name: "T"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertProduct(ctx, domain.Product{
		ID: tenantID + "/" + sku, TenantID: tenantID, SKU: sku, Name: "P", BaseYen: baseYen,
	}); err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	// SKU-specific rule row.
	if err := store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-overlap", TenantID: tenantID, Name: "Overlap SKU",
		Type: domain.PromotionPercent, Value: pct, Channel: "web",
		SKU: sku, Priority: 5, ValidFrom: from, ValidUntil: until,
	}); err != nil {
		t.Fatal(err)
	}
	// Channel-wildcard rule row (same id, empty SKU).
	if err := store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-overlap", TenantID: tenantID, Name: "Overlap wildcard",
		Type: domain.PromotionPercent, Value: pct, Channel: "web",
		SKU: "", Priority: 5, ValidFrom: from, ValidUntil: until,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestDuplicateDiscount_InteractivePath verifies that the interactive
// pricing engine applies the overlapping promotion exactly once.
func TestDuplicateDiscount_InteractivePath(t *testing.T) {
	store := storage.NewMemoryStore()
	seedOverlapPromotion(t, store, "tenant-a", "SKU-00001", 1000, 10.0)
	logger := observability.Default("test")
	eng := pricing.NewEngine(store, logger, pricing.NewResultCache(64))
	d, err := eng.Quote(context.Background(), domain.PricingRequest{
		TenantID: "tenant-a", CustomerID: "cust-001", SKU: "SKU-00001",
		Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if len(d.AppliedIDs) != 1 {
		t.Fatalf("expected exactly 1 applied id, got %d (%v)", len(d.AppliedIDs), d.AppliedIDs)
	}
	if d.FinalYen != 900 {
		t.Fatalf("expected 900 (1000 - 10%%), got %d", d.FinalYen)
	}
}

// TestDuplicateDiscount_BatchPath verifies that the batch pricing engine
// applies the overlapping promotion exactly once.
func TestDuplicateDiscount_BatchPath(t *testing.T) {
	store := storage.NewMemoryStore()
	seedOverlapPromotion(t, store, "tenant-a", "SKU-00001", 1000, 10.0)
	logger := observability.Default("test")
	be := pricing.NewBatchEngine(store, logger, pricing.NewResultCache(64))
	results, err := be.Run(context.Background(), "tenant-a", []domain.PricingRequest{
		{TenantID: "tenant-a", CustomerID: "cust-001", SKU: "SKU-00001",
			Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	d := results[0].Decision
	if len(d.AppliedIDs) != 1 {
		t.Fatalf("expected exactly 1 applied id, got %d (%v)", len(d.AppliedIDs), d.AppliedIDs)
	}
	if d.FinalYen != 900 {
		t.Fatalf("expected 900, got %d", d.FinalYen)
	}
}

// TestDuplicateDiscount_InteractiveAndBatchAgree verifies that the
// interactive and batch paths produce the same final amount for the
// same overlapping input.
func TestDuplicateDiscount_InteractiveAndBatchAgree(t *testing.T) {
	store := storage.NewMemoryStore()
	seedOverlapPromotion(t, store, "tenant-a", "SKU-00001", 1000, 10.0)
	logger := observability.Default("test")
	cache := pricing.NewResultCache(64)
	intEng := pricing.NewEngine(store, logger, cache)
	batchEng := pricing.NewBatchEngine(store, logger, cache)
	req := domain.PricingRequest{
		TenantID: "tenant-a", CustomerID: "cust-001", SKU: "SKU-00001",
		Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	intDec, err := intEng.Quote(context.Background(), req)
	if err != nil {
		t.Fatalf("interactive: %v", err)
	}
	batch, err := batchEng.Run(context.Background(), "tenant-a", []domain.PricingRequest{req})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if intDec.FinalYen != batch[0].Decision.FinalYen {
		t.Fatalf("interactive/batch disagree: %d vs %d", intDec.FinalYen, batch[0].Decision.FinalYen)
	}
	if intDec.FinalYen != 900 {
		t.Fatalf("expected 900, got %d", intDec.FinalYen)
	}
}

// TestDuplicateDiscount_WildcardOnly verifies that a single wildcard
// promotion is applied exactly once.
func TestDuplicateDiscount_WildcardOnly(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "T"})
	_ = store.UpsertProduct(ctx, domain.Product{ID: "tenant-a/SKU-00001", TenantID: "tenant-a", SKU: "SKU-00001", Name: "P", BaseYen: 1000})
	_ = store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-wild", TenantID: "tenant-a", Name: "Wildcard",
		Type: domain.PromotionPercent, Value: 5.0, Channel: "web",
		SKU: "", Priority: 5,
		ValidFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ValidUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	logger := observability.Default("test")
	eng := pricing.NewEngine(store, logger, pricing.NewResultCache(64))
	d, err := eng.Quote(ctx, domain.PricingRequest{
		TenantID: "tenant-a", CustomerID: "cust-001", SKU: "SKU-00001",
		Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if len(d.AppliedIDs) != 1 {
		t.Fatalf("expected 1 applied, got %d", len(d.AppliedIDs))
	}
	if d.FinalYen != 950 {
		t.Fatalf("expected 950, got %d", d.FinalYen)
	}
}

// TestDuplicateDiscount_RepeatedPath verifies that the same promotion
// id registered with two rule rows (SKU-specific and channel-wildcard)
// is still applied exactly once even when the resolver iterates both
// paths.
func TestDuplicateDiscount_RepeatedPath(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "T"})
	_ = store.UpsertProduct(ctx, domain.Product{ID: "tenant-a/SKU-00001", TenantID: "tenant-a", SKU: "SKU-00001", Name: "P", BaseYen: 1000})
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	// Two rule rows for the same promotion id: SKU-specific and wildcard.
	_ = store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-repeat", TenantID: "tenant-a", Name: "Repeat SKU",
		Type: domain.PromotionPercent, Value: 10.0, Channel: "web",
		SKU: "SKU-00001", Priority: 5, ValidFrom: from, ValidUntil: until,
	})
	_ = store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-repeat", TenantID: "tenant-a", Name: "Repeat wildcard",
		Type: domain.PromotionPercent, Value: 10.0, Channel: "web",
		SKU: "", Priority: 5, ValidFrom: from, ValidUntil: until,
	})
	logger := observability.Default("test")
	eng := pricing.NewEngine(store, logger, pricing.NewResultCache(64))
	d, err := eng.Quote(ctx, domain.PricingRequest{
		TenantID: "tenant-a", CustomerID: "cust-001", SKU: "SKU-00001",
		Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if len(d.AppliedIDs) != 1 {
		t.Fatalf("expected 1 applied, got %d (%v)", len(d.AppliedIDs), d.AppliedIDs)
	}
	if d.FinalYen != 900 {
		t.Fatalf("expected 900, got %d", d.FinalYen)
	}
}

// TestDuplicateDiscount_OverlappingRulesDifferentPromos verifies that
// two DIFFERENT promotion ids (one SKU-specific, one wildcard) are
// both applied — the dedup is by id, not by path.
func TestDuplicateDiscount_OverlappingRulesDifferentPromos(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "T"})
	_ = store.UpsertProduct(ctx, domain.Product{ID: "tenant-a/SKU-00001", TenantID: "tenant-a", SKU: "SKU-00001", Name: "P", BaseYen: 1000})
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-sku", TenantID: "tenant-a", Name: "SKU",
		Type: domain.PromotionPercent, Value: 5.0, Channel: "web",
		SKU: "SKU-00001", Priority: 10, ValidFrom: from, ValidUntil: until,
	})
	_ = store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-wild", TenantID: "tenant-a", Name: "Wild",
		Type: domain.PromotionPercent, Value: 5.0, Channel: "web",
		SKU: "", Priority: 5, ValidFrom: from, ValidUntil: until,
	})
	logger := observability.Default("test")
	eng := pricing.NewEngine(store, logger, pricing.NewResultCache(64))
	d, err := eng.Quote(ctx, domain.PricingRequest{
		TenantID: "tenant-a", CustomerID: "cust-001", SKU: "SKU-00001",
		Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if len(d.AppliedIDs) != 2 {
		t.Fatalf("expected 2 applied (different ids), got %d (%v)", len(d.AppliedIDs), d.AppliedIDs)
	}
	// 1000 * 0.95 * 0.95 = 902.5 -> 903 (half-up rounding).
	if d.FinalYen != 903 {
		t.Fatalf("expected 903, got %d", d.FinalYen)
	}
}
