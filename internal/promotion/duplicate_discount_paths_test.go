// Package promotion — duplicate_discount_paths_test.go
package promotion

import (
	"testing"
	"time"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/money"
)

func TestDuplicateDiscount_Interactive(t *testing.T) {
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	base := money.FromFloatYen(3504)
	candidates := []domain.Promotion{
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 7.5, Channel: "web", SKU: "SKU-00020", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 5.2, Channel: "web", SKU: "", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
	}
	applied, _, _, _, _ := Apply(candidates, base, 1, "SKU-00020", at)
	count := 0
	for _, id := range applied {
		if id == "promo-0004" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("interactive: promo-0004 applied %d times, want 1", count)
	}
}

func TestDuplicateDiscount_Batch(t *testing.T) {
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	base := money.FromFloatYen(3504)
	candidates := []domain.Promotion{
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 7.5, Channel: "web", SKU: "SKU-00020", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 5.2, Channel: "web", SKU: "", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
	}
	applied, _, _, _, _ := Apply(candidates, base, 1, "SKU-00020", at)
	count := 0
	for _, id := range applied {
		if id == "promo-0004" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("batch: promo-0004 applied %d times, want 1", count)
	}
}

func TestDuplicateDiscount_OverlappingRules(t *testing.T) {
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	base := money.FromFloatYen(3504)
	candidates := []domain.Promotion{
		{ID: "promo-A", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 5, Channel: "web", SKU: "SKU-00020", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
		{ID: "promo-B", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 3, Channel: "web", SKU: "SKU-00020", Priority: 6, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
	}
	applied, _, _, _, _ := Apply(candidates, base, 1, "SKU-00020", at)
	if len(applied) != 2 {
		t.Fatalf("overlapping: applied %d promotions, want 2", len(applied))
	}
}

func TestDuplicateDiscount_WildcardOnly(t *testing.T) {
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	base := money.FromFloatYen(3504)
	candidates := []domain.Promotion{
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 5.2, Channel: "web", SKU: "", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
	}
	applied, _, _, _, _ := Apply(candidates, base, 1, "SKU-00020", at)
	count := 0
	for _, id := range applied {
		if id == "promo-0004" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("wildcard: promo-0004 applied %d times, want 1", count)
	}
}

func TestDuplicateDiscount_RepeatedPath(t *testing.T) {
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	base := money.FromFloatYen(3504)
	candidates := []domain.Promotion{
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 7.5, Channel: "web", SKU: "SKU-00020", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 5.2, Channel: "web", SKU: "", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionAmount, Value: 217, Channel: "web", SKU: "SKU-00046", Priority: 6, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
	}
	applied, _, _, _, _ := Apply(candidates, base, 1, "SKU-00020", at)
	count := 0
	for _, id := range applied {
		if id == "promo-0004" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("repeated-path: promo-0004 applied %d times, want 1", count)
	}
}
