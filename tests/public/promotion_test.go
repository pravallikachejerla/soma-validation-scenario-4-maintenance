package public

import (
	"context"
	"testing"
	"time"

	"github.com/somagen/scenario4/internal/application"
	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/storage"
)

func TestPromotion_CRUD(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "T"})
	ps := application.NewPricingService(store, nil, nil)
	p := domain.Promotion{
		ID:         "promo-1",
		TenantID:   "tenant-a",
		Name:       "P",
		Type:       domain.PromotionPercent,
		Value:      10.0,
		Channel:    "web",
		Priority:   1,
		ValidFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ValidUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := ps.CreatePromotion(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := ps.ListPromotions(ctx, "tenant-a", "web")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != "promo-1" {
		t.Fatalf("unexpected list: %+v", got)
	}
	p.Value = 20
	if err := ps.UpdatePromotion(ctx, p); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = ps.ListPromotions(ctx, "tenant-a", "web")
	if got[0].Value != 20 {
		t.Fatalf("update not applied: %+v", got[0])
	}
}

func TestPromotion_ListIsChannelScoped(t *testing.T) {
	store := storage.NewMemoryStore()
	ctx := context.Background()
	_ = store.UpsertTenant(ctx, domain.Tenant{ID: "tenant-a", Name: "T"})
	ps := application.NewPricingService(store, nil, nil)
	for _, ch := range []string{"web", "store", "mobile"} {
		_ = ps.CreatePromotion(ctx, domain.Promotion{
			ID:         "promo-" + ch,
			TenantID:   "tenant-a",
			Name:       "P",
			Type:       domain.PromotionPercent,
			Value:      10.0,
			Channel:    ch,
			ValidFrom:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			ValidUntil: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		})
	}
	got, err := ps.ListPromotions(ctx, "tenant-a", "web")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1, got %d", len(got))
	}
	if got[0].Channel != "web" {
		t.Fatalf("wrong channel: %s", got[0].Channel)
	}
}
