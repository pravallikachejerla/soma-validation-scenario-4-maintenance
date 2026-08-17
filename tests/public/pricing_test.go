// Package public contains the public test suite for the Scenario 4
// pricing application. These tests exercise normal engineering
// expectations and MUST pass on the seeded source. They are kept
// entirely independent of any planted condition: no overlap fixtures,
// no internal package, no audit-side-effects they care about.
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

func newEngine(t *testing.T) (*pricing.Engine, storage.Store) {
	t.Helper()
	store := storage.NewMemoryStore()
	ctx := context.Background()
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "Synthetic tenant-a"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertProduct(ctx, domain.Product{
		ID:       "tenant-a/SKU-00001",
		TenantID: "tenant-a",
		SKU:      "SKU-00001",
		Name:     "Product SKU-00001",
		BaseYen:  1000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPromotion(ctx, domain.Promotion{
		ID:         "promo-public-1",
		TenantID:   "tenant-a",
		Name:       "Public percent promo",
		Type:       domain.PromotionPercent,
		Value:      10.0,
		Channel:    "web",
		Priority:   5,
		ValidFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ValidUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	logger := observability.Default("test")
	eng := pricing.NewEngine(store, logger, pricing.NewResultCache(64))
	return eng, store
}

func TestInteractivePricing_RoundTrip(t *testing.T) {
	eng, _ := newEngine(t)
	d, err := eng.Quote(context.Background(), domain.PricingRequest{
		TenantID:   "tenant-a",
		CustomerID: "cust-001",
		SKU:        "SKU-00001",
		Quantity:   1,
		Channel:    "web",
		At:         time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if d.FinalYen != 900 {
		t.Fatalf("expected 900, got %d", d.FinalYen)
	}
	if len(d.AppliedIDs) != 1 {
		t.Fatalf("expected 1 applied, got %d", len(d.AppliedIDs))
	}
}

func TestInteractiveAndBatchAgree(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	if err := store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "Synthetic tenant-a"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertProduct(ctx, domain.Product{
		ID: "tenant-a/SKU-00001", TenantID: "tenant-a", SKU: "SKU-00001", Name: "P", BaseYen: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-public-1", TenantID: "tenant-a", Name: "P", Type: domain.PromotionPercent,
		Value: 10.0, Channel: "web", Priority: 5,
		ValidFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ValidUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	logger := observability.Default("test")
	cache := pricing.NewResultCache(64)
	intEng := pricing.NewEngine(store, logger, cache)
	batchEng := pricing.NewBatchEngine(store, logger, cache)

	req := domain.PricingRequest{
		TenantID: "tenant-a", CustomerID: "cust-001", SKU: "SKU-00001",
		Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	intDec, err := intEng.Quote(ctx, req)
	if err != nil {
		t.Fatalf("interactive: %v", err)
	}
	batch, err := batchEng.Run(ctx, "tenant-a", []domain.PricingRequest{req})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if intDec.FinalYen != batch[0].Decision.FinalYen {
		t.Fatalf("interactive/batch disagree: %d vs %d", intDec.FinalYen, batch[0].Decision.FinalYen)
	}
}

func TestPromotion_InclusiveEndTime(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "T"})
	_ = store.UpsertProduct(ctx, domain.Product{ID: "tenant-a/SKU-00001", TenantID: "tenant-a", SKU: "SKU-00001", Name: "P", BaseYen: 1000})
	_ = store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-end", TenantID: "tenant-a", Name: "P", Type: domain.PromotionPercent,
		Value: 10.0, Channel: "web", Priority: 5,
		ValidFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ValidUntil: time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC),
	})
	logger := observability.Default("test")
	eng := pricing.NewEngine(store, logger, pricing.NewResultCache(64))
	// Exactly at the end-of-window timestamp: still active because the
	// comparison is inclusive (<=).
	d, err := eng.Quote(ctx, domain.PricingRequest{
		TenantID: "tenant-a", CustomerID: "cust-001", SKU: "SKU-00001",
		Quantity: 1, Channel: "web", At: time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if d.FinalYen != 900 {
		t.Fatalf("expected inclusive-end behaviour, got %d", d.FinalYen)
	}
}

func TestPromotion_NoOverlap(t *testing.T) {
	// Public test: with a single wildcard promotion, the resolver
	// returns exactly one applied id. (The overlap case lives in the
	// private test package.)
	store := storage.NewMemoryStore()
	ctx := context.Background()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "T"})
	_ = store.UpsertProduct(ctx, domain.Product{ID: "tenant-a/SKU-00001", TenantID: "tenant-a", SKU: "SKU-00001", Name: "P", BaseYen: 1000})
	_ = store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-wild", TenantID: "tenant-a", Name: "P", Type: domain.PromotionPercent,
		Value: 5.0, Channel: "web", Priority: 5,
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
		t.Fatalf("expected exactly 1 applied, got %d", len(d.AppliedIDs))
	}
	if d.FinalYen != 950 {
		t.Fatalf("expected 950, got %d", d.FinalYen)
	}
}
