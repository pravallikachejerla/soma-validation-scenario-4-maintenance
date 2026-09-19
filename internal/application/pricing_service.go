// Package application contains the use-case-level services. They sit
// between the HTTP transport and the storage layer.
package application

import (
	"context"
	"time"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/pricing"
	"github.com/somagen/scenario4/internal/storage"
)

// PricingService is the application-level service used by HTTP handlers.
type PricingService struct {
	Store       storage.Store
	Engine      *pricing.Engine
	BatchEngine *pricing.BatchEngine
	Logger      *observability.Logger
	Metrics     *observability.Metrics
}

// NewPricingService wires a PricingService. The override service is
// optional — when present, the pricing engine will honour applied
// overrides for any (tenant, sku, channel, at) match.
func NewPricingService(s storage.Store, l *observability.Logger, m *observability.Metrics, ov *OverrideService) *PricingService {
	cache := pricing.NewResultCache(256)
	eng := pricing.NewEngine(s, l, cache)
	if ov != nil {
		eng.OverrideLookup = pricingOverrideLookup{ov: ov}
	}
	return &PricingService{
		Store:       s,
		Engine:      eng,
		BatchEngine: pricing.NewBatchEngine(s, l, cache),
		Logger:      l,
		Metrics:     m,
	}
}

// pricingOverrideLookup adapts application.OverrideService to the
// pricing.OverrideLookup interface. Keeping this adapter here, instead
// of in the pricing package, preserves the layering rule that pricing
// must not import application.
type pricingOverrideLookup struct {
	ov *OverrideService
}

func (p pricingOverrideLookup) FindActive(ctx context.Context, tenantID, sku, channel string, at time.Time) (int64, bool, error) {
	o, ok, err := p.ov.FindActiveOverride(ctx, tenantID, sku, channel, at)
	if err != nil {
		return 0, false, err
	}
	if !ok {
		return 0, false, nil
	}
	return o.OverrideYen, true, nil
}

// ListPromotions returns active promotions for a tenant and channel.
func (p *PricingService) ListPromotions(ctx context.Context, tenantID, channel string) ([]domain.Promotion, error) {
	return p.Store.ListPromotions(ctx, tenantID, channel)
}

// CreatePromotion registers a new promotion.
func (p *PricingService) CreatePromotion(ctx context.Context, promo domain.Promotion) error {
	if err := p.Store.UpsertPromotion(ctx, promo); err != nil {
		return err
	}
	_, _ = p.Store.BumpConfigVersion(ctx, promo.TenantID)
	_ = p.Store.AppendAudit(ctx, domain.AuditEvent{
		ID:       "audit-" + promo.ID,
		TenantID: promo.TenantID,
		Action:   "promotion.created",
		Subject:  promo.ID,
		Detail:   "name=" + promo.Name + "; channel=" + promo.Channel,
	})
	return nil
}

// UpdatePromotion updates a promotion and bumps the config version.
func (p *PricingService) UpdatePromotion(ctx context.Context, promo domain.Promotion) error {
	if err := p.Store.UpdatePromotion(ctx, promo); err != nil {
		return err
	}
	_, _ = p.Store.BumpConfigVersion(ctx, promo.TenantID)
	_ = p.Store.AppendAudit(ctx, domain.AuditEvent{
		ID:       "audit-" + promo.ID,
		TenantID: promo.TenantID,
		Action:   "promotion.updated",
		Subject:  promo.ID,
		Detail:   "name=" + promo.Name + "; channel=" + promo.Channel,
	})
	return nil
}

// Quote evaluates a single pricing request.
func (p *PricingService) Quote(ctx context.Context, req domain.PricingRequest) (domain.PricingDecision, error) {
	return p.Engine.Quote(ctx, req)
}

// Batch evaluates a list of pricing requests.
func (p *PricingService) Batch(ctx context.Context, tenantID string, requests []domain.PricingRequest) ([]pricing.BatchResult, error) {
	return p.BatchEngine.Run(ctx, tenantID, requests)
}

// ListAudit returns recent audit events for a tenant.
func (p *PricingService) ListAudit(ctx context.Context, tenantID string, limit int) ([]domain.AuditEvent, error) {
	return p.Store.ListAudit(ctx, tenantID, limit)
}

// ListProducts returns products for a tenant.
func (p *PricingService) ListProducts(ctx context.Context, tenantID string) ([]domain.Product, error) {
	return p.Store.ListProducts(ctx, tenantID)
}

// Now is a small helper for the seeder.
func Now() time.Time { return time.Now().UTC() }
