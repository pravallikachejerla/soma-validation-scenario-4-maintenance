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

func TestBatch_ProducesStableHash(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "T"})
	_ = store.UpsertProduct(ctx, domain.Product{ID: "tenant-a/SKU-00001", TenantID: "tenant-a", SKU: "SKU-00001", Name: "P", BaseYen: 1000})
	_ = store.UpsertProduct(ctx, domain.Product{ID: "tenant-a/SKU-00002", TenantID: "tenant-a", SKU: "SKU-00002", Name: "P", BaseYen: 2000})
	logger := observability.Default("test")
	be := pricing.NewBatchEngine(store, logger, pricing.NewResultCache(64))

	requests := []domain.PricingRequest{
		{TenantID: "tenant-a", SKU: "SKU-00001", Channel: "web", Quantity: 1, At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
		{TenantID: "tenant-a", SKU: "SKU-00002", Channel: "web", Quantity: 1, At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
	}
	first, err := be.Run(ctx, "tenant-a", requests)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	second, err := be.Run(ctx, "tenant-a", requests)
	if err != nil {
		t.Fatalf("batch 2: %v", err)
	}
	if pricing.ResultHash(first) != pricing.ResultHash(second) {
		t.Fatalf("expected stable hash")
	}
}

func TestBatch_EmptyInput(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	logger := observability.Default("test")
	be := pricing.NewBatchEngine(store, logger, pricing.NewResultCache(8))
	results, err := be.Run(ctx, "tenant-a", nil)
	if err != nil {
		t.Fatalf("batch empty: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty, got %d", len(results))
	}
}
