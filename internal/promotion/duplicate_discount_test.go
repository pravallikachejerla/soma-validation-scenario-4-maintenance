// Package promotion — duplicate_discount_test.go
//
// Regression test for the duplicate-discount defect.
//
// Defect: when a promotion id is registered with BOTH a SKU-specific rule
// row (sku='SKU-00020') AND a channel-wildcard rule row (sku=”), the
// storage layer returns the same id twice. The Apply loop in resolver.go
// walks the merged list and applies the discount twice, halving the final
// price (or worse, for percent promotions compounding multiplicatively).
//
// This test pins the expected behaviour: a promotion id must be applied at
// most once per quote, regardless of how many rule rows match.
package promotion

import (
	"testing"
	"time"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/money"
)

// TestDuplicateDiscount_DualPathPromotion proves the defect.
//
// Seed: promo-0004 has three rule rows for tenant-a / channel=web:
//   - sku='SKU-00020', priority=7, percent=7.595...
//   - sku=”,          priority=7, percent=5.247...
//   - sku='SKU-00046', priority=6, amount=217
//
// For a quote on SKU-00020 / channel=web, the SKU-specific row matches
// the request SKU and the wildcard row matches the channel. Both rows
// carry the same promotion id (promo-0004). The current Apply loop
// applies the discount twice.
//
// Expected (post-fix): promo-0004 appears exactly once in appliedIDs.
// Actual (pre-fix):    promo-0004 appears twice.
func TestDuplicateDiscount_DualPathPromotion(t *testing.T) {
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	base := money.FromFloatYen(3504)

	candidates := []domain.Promotion{
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 7.595340683368084, Channel: "web", SKU: "SKU-00020", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 5.247647026085712, Channel: "web", SKU: "", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionAmount, Value: 217, Channel: "web", SKU: "SKU-00046", Priority: 6, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
	}

	applied, _, _, final, _ := Apply(candidates, base, 1, "SKU-00020", at)

	count := 0
	for _, id := range applied {
		if id == "promo-0004" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("duplicate-discount defect: promo-0004 applied %d times, want exactly 1 (applied=%v, final=%s)", count, applied, final.String())
	}
}

// TestDuplicateDiscount_OnlyWildcardMatches is the control case: when the
// request SKU does not match any SKU-specific row, only the wildcard row
// applies. The promotion must still appear exactly once.
func TestDuplicateDiscount_OnlyWildcardMatches(t *testing.T) {
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	base := money.FromFloatYen(1000)

	candidates := []domain.Promotion{
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 10, Channel: "web", SKU: "SKU-00020", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
		{ID: "promo-0004", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 5, Channel: "web", SKU: "", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
	}

	applied, _, _, _, _ := Apply(candidates, base, 1, "SKU-99999", at)

	count := 0
	for _, id := range applied {
		if id == "promo-0004" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("wildcard-only path: promo-0004 applied %d times, want exactly 1 (applied=%v)", count, applied)
	}
}

// TestDuplicateDiscount_DistinctIDsUnaffected proves the fix does not
// over-deduplicate: two different promotion ids must both apply.
func TestDuplicateDiscount_DistinctIDsUnaffected(t *testing.T) {
	at := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	base := money.FromFloatYen(1000)

	candidates := []domain.Promotion{
		{ID: "promo-A", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 10, Channel: "web", SKU: "SKU-00020", Priority: 7, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
		{ID: "promo-B", TenantID: "tenant-a", Type: domain.PromotionPercent, Value: 5, Channel: "web", SKU: "", Priority: 6, ValidFrom: at.Add(-24 * time.Hour), ValidUntil: at.Add(24 * time.Hour)},
	}

	applied, _, _, _, _ := Apply(candidates, base, 1, "SKU-00020", at)

	if len(applied) != 2 {
		t.Fatalf("distinct ids: got %d applications, want 2 (applied=%v)", len(applied), applied)
	}
}
