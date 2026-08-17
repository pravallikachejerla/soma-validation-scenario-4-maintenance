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
	Store        storage.Store
	Engine       *pricing.Engine
	BatchEngine  *pricing.BatchEngine
	Logger       *observability.Logger
	Metrics      *observability.Metrics
}

// NewPricingService wires a PricingService.
func NewPricingService(s storage.Store, l *observability.Logger, m *observability.Metrics) *PricingService {
	cache := pricing.NewResultCache(256)
	return &PricingService{
		Store:       s,
		Engine:      pricing.NewEngine(s, l, cache),
		BatchEngine: pricing.NewBatchEngine(s, l, cache),
		Logger:      l,
		Metrics:     m,
	}
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
