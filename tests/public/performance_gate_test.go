// Package public — performance_gate_test.go
//
// Performance gate tests for the pricing engine and override workflow.
// These tests verify that:
//   - The interactive pricing engine handles a burst of requests
//     within a reasonable time budget.
//   - The batch engine processes a batch of requests efficiently.
//   - The override workflow handles a burst of requests without
//     degradation.
package public

import (
	"context"
	"testing"
	"time"

	"github.com/somagen/scenario4/internal/application"
	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/pricing"
	"github.com/somagen/scenario4/internal/storage"
)

// TestPerformance_InteractivePricingBurst verifies that the
// interactive pricing engine handles 1000 requests within 5 seconds.
func TestPerformance_InteractivePricingBurst(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	store := storage.NewMemoryStore()
	ctx := context.Background()

	// Seed.
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "T"})
	_ = store.UpsertProduct(ctx, domain.Product{
		ID: "tenant-a/SKU-00001", TenantID: "tenant-a", SKU: "SKU-00001",
		Name: "P", BaseYen: 1000,
	})
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-perf", TenantID: "tenant-a", Name: "Perf",
		Type: domain.PromotionPercent, Value: 10.0, Channel: "web",
		SKU: "", Priority: 5, ValidFrom: from, ValidUntil: until,
	})

	logger := observability.Default("test")
	eng := pricing.NewEngine(store, logger, pricing.NewResultCache(256))

	start := time.Now()
	for i := 0; i < 1000; i++ {
		_, err := eng.Quote(ctx, domain.PricingRequest{
			TenantID: "tenant-a", CustomerID: "cust-001", SKU: "SKU-00001",
			Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("quote %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	// 1000 requests should complete in under 5 seconds.
	if elapsed > 5*time.Second {
		t.Fatalf("1000 requests took %v, expected < 5s", elapsed)
	}
	t.Logf("1000 interactive requests completed in %v (%.2f req/ms)", elapsed, 1000.0/float64(elapsed.Milliseconds()))
}

// TestPerformance_BatchPricingEfficiency verifies that the batch
// engine processes 500 requests efficiently.
func TestPerformance_BatchPricingEfficiency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	store := storage.NewMemoryStore()
	ctx := context.Background()

	// Seed.
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "T"})
	_ = store.UpsertProduct(ctx, domain.Product{
		ID: "tenant-a/SKU-00001", TenantID: "tenant-a", SKU: "SKU-00001",
		Name: "P", BaseYen: 1000,
	})
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = store.UpsertPromotion(ctx, domain.Promotion{
		ID: "promo-perf", TenantID: "tenant-a", Name: "Perf",
		Type: domain.PromotionPercent, Value: 10.0, Channel: "web",
		SKU: "", Priority: 5, ValidFrom: from, ValidUntil: until,
	})

	logger := observability.Default("test")
	be := pricing.NewBatchEngine(store, logger, pricing.NewResultCache(256))

	requests := make([]domain.PricingRequest, 500)
	for i := range requests {
		requests[i] = domain.PricingRequest{
			TenantID: "tenant-a", CustomerID: "cust-001", SKU: "SKU-00001",
			Quantity: 1, Channel: "web", At: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		}
	}

	start := time.Now()
	results, err := be.Run(ctx, "tenant-a", requests)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if len(results) != 500 {
		t.Fatalf("expected 500 results, got %d", len(results))
	}

	// 500 batch requests should complete in under 5 seconds.
	if elapsed > 5*time.Second {
		t.Fatalf("500 batch requests took %v, expected < 5s", elapsed)
	}
	t.Logf("500 batch requests completed in %v", elapsed)
}

// TestPerformance_OverrideWorkflowBurst verifies that the override
// workflow handles a burst of requests without degradation.
func TestPerformance_OverrideWorkflowBurst(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	store := storage.NewMemoryStore()
	ctx := context.Background()
	logger := observability.Default("test")

	_ = store.UpsertTenant(ctx, domain.Tenant{
		ID: "tenant-a", Name: "T", OverrideEnabled: true,
	})

	svc := application.NewOverrideService(store, logger)

	start := time.Now()
	for i := 0; i < 100; i++ {
		_, err := svc.CreateOverrideRequest(ctx,
			"tenant-a", "user-requester", "SKU-00001", "web", "cust-001",
			1000, 500, "perf test")
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	// 100 override requests should complete in under 5 seconds.
	if elapsed > 5*time.Second {
		t.Fatalf("100 override requests took %v, expected < 5s", elapsed)
	}
	t.Logf("100 override requests completed in %v", elapsed)
}
