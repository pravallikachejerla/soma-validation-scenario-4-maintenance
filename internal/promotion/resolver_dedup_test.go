package promotion

import (
	"testing"
	"time"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/money"
)

// activeWindow returns a far-future-inclusive validity window so every
// test promotion is active for any reasonable "at" time.
func activeWindow() (time.Time, time.Time) {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
}

// TestApply_DeduplicatesByPromotionID_PromotionD1 is the failing-test-first
// proof for the defect correction. A single promotion id registered with
// two rule rows (one SKU-specific, one channel-wildcard) MUST be applied
// exactly once when the resolver merges the two paths.
func TestApply_DeduplicatesByPromotionID_PromotionD1(t *testing.T) {
	from, until := activeWindow()
	candidates := []domain.Promotion{
		{
			ID: "promo-overlap", TenantID: "tenant-a", Name: "Overlap",
			Type: domain.PromotionPercent, Value: 10.0,
			Channel: "web", SKU: "SKU-00001", Priority: 5,
			ValidFrom: from, ValidUntil: until,
		},
		{
			ID: "promo-overlap", TenantID: "tenant-a", Name: "Overlap",
			Type: domain.PromotionPercent, Value: 10.0,
			Channel: "web", SKU: "", Priority: 5,
			ValidFrom: from, ValidUntil: until,
		},
	}
	applied, audit, _, final, _ := Apply(
		candidates, money.FromYen(1000), 1, "SKU-00001",
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)
	if len(applied) != 1 {
		t.Fatalf("expected exactly 1 application, got %d: %v", len(applied), applied)
	}
	if len(audit) != 1 {
		t.Fatalf("expected exactly 1 audit id, got %d", len(audit))
	}
	if final.Yen() != 900 {
		t.Fatalf("expected 900 (1000 * 0.9), got %d", final.Yen())
	}
}

// TestApply_DeduplicatesByPromotionID_PrefersSKUWhenPresent exercises the
// priority rule: when BOTH a SKU-specific rule and a channel-wildcard
// rule exist for the same promotion id, the SKU-specific rule wins
// (it is more specific) — the resolver applies it, not the wildcard.
func TestApply_DeduplicatesByPromotionID_PrefersSKUWhenPresent(t *testing.T) {
	from, until := activeWindow()
	candidates := []domain.Promotion{
		{
			ID: "promo-mixed", TenantID: "tenant-a", Name: "SKU-specific 5%",
			Type: domain.PromotionPercent, Value: 5.0,
			Channel: "web", SKU: "SKU-00001", Priority: 5,
			ValidFrom: from, ValidUntil: until,
		},
		{
			ID: "promo-mixed", TenantID: "tenant-a", Name: "Wildcard 20%",
			Type: domain.PromotionPercent, Value: 20.0,
			Channel: "web", SKU: "", Priority: 5,
			ValidFrom: from, ValidUntil: until,
		},
	}
	applied, _, _, final, _ := Apply(
		candidates, money.FromYen(1000), 1, "SKU-00001",
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)
	if len(applied) != 1 {
		t.Fatalf("expected exactly 1 application, got %d", len(applied))
	}
	// The SKU-specific 5% rule was applied, NOT the wildcard 20% rule.
	if final.Yen() != 950 {
		t.Fatalf("expected 950 (5%% off), got %d (wildcard would have given 800)", final.Yen())
	}
}

// TestApply_KeepsDistinctPromotions confirms de-duplication does NOT
// collapse two genuinely different promotion ids into one application.
func TestApply_KeepsDistinctPromotions(t *testing.T) {
	from, until := activeWindow()
	candidates := []domain.Promotion{
		{
			ID: "promo-a", TenantID: "tenant-a", Name: "A",
			Type: domain.PromotionPercent, Value: 5.0,
			Channel: "web", SKU: "SKU-00001", Priority: 10,
			ValidFrom: from, ValidUntil: until,
		},
		{
			ID: "promo-b", TenantID: "tenant-a", Name: "B",
			Type: domain.PromotionPercent, Value: 5.0,
			Channel: "web", SKU: "", Priority: 5,
			ValidFrom: from, ValidUntil: until,
		},
	}
	applied, _, _, final, _ := Apply(
		candidates, money.FromYen(1000), 1, "SKU-00001",
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	)
	if len(applied) != 2 {
		t.Fatalf("expected 2 applications (distinct ids), got %d", len(applied))
	}
	// 5% off, then 5% off: 1000 * 0.95 * 0.95 = 902.5 -> 903.
	if final.Yen() != 903 {
		t.Fatalf("expected 903 (stacked 5%% + 5%%), got %d", final.Yen())
	}
}
