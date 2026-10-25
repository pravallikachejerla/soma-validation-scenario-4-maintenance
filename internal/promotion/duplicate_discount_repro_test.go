// Package promotion — duplicate_discount_repro_test.go
//
// Failing reproduction test for the duplicate-discount defect.
package promotion

import (
	"testing"
	"time"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/money"
)

func TestDuplicateDiscount_Repro_FailsOnUnfixedSource(t *testing.T) {
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
