package application

import (
	"context"
	"time"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/override"
	"github.com/somagen/scenario4/internal/storage"
)

// OverrideService is the application-layer facade for the manual
// price-override approval workflow. It adapts the in-process
// override.Service to the storage.Store backend.
//
// Sits in the application layer so that:
//   - the override package remains pure / I/O-free
//   - the HTTP layer depends on the application boundary, not on internal/override directly
type OverrideService struct {
	inner *override.Service
	store storage.Store
}

// NewOverrideService constructs a service that uses the given store
// as its persistence backend.
func NewOverrideService(s storage.Store) *OverrideService {
	return &OverrideService{
		inner: override.NewService(storeAdapter{s}, func() time.Time { return time.Now().UTC() }),
		store: s,
	}
}

// storeAdapter wraps storage.Store to satisfy override.Repo. The
// override package exposes a narrower Repo (no ListOverrides filter),
// so we route the listing verb from the storage API.
type storeAdapter struct{ s storage.Store }

func (a storeAdapter) CreateOverride(ctx context.Context, o domain.Override) error {
	return a.s.CreateOverride(ctx, o)
}

func (a storeAdapter) GetOverride(ctx context.Context, id string) (domain.Override, error) {
	return a.s.GetOverride(ctx, id)
}

func (a storeAdapter) UpdateOverride(ctx context.Context, o domain.Override) error {
	return a.s.UpdateOverride(ctx, o)
}

func (a storeAdapter) ListOverridesByTenant(ctx context.Context, tenantID string) ([]domain.Override, error) {
	return a.s.ListOverridesByTenant(ctx, tenantID)
}

func (a storeAdapter) GetThreshold(ctx context.Context, tenantID string) (domain.OverrideThreshold, error) {
	return a.s.GetThreshold(ctx, tenantID)
}

func (a storeAdapter) UpsertThreshold(ctx context.Context, t domain.OverrideThreshold) error {
	return a.s.UpsertThreshold(ctx, t)
}

func (a storeAdapter) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	return a.s.AppendAudit(ctx, e)
}

func (a storeAdapter) BumpConfigVersion(ctx context.Context, tenantID string) (int64, error) {
	return a.s.BumpConfigVersion(ctx, tenantID)
}

// Request delegates to the inner service.
func (o *OverrideService) Request(ctx context.Context, in override.RequestInput) (domain.Override, error) {
	return o.inner.Request(ctx, in)
}

// FirstApprove delegates to the inner service.
func (o *OverrideService) FirstApprove(ctx context.Context, id, approverID string) (domain.Override, error) {
	return o.inner.FirstApprove(ctx, id, approverID)
}

// SecondApprove delegates to the inner service.
func (o *OverrideService) SecondApprove(ctx context.Context, id, approverID string) (domain.Override, error) {
	return o.inner.SecondApprove(ctx, id, approverID)
}

// Reject delegates to the inner service.
func (o *OverrideService) Reject(ctx context.Context, id, approverID, reason string) (domain.Override, error) {
	return o.inner.Reject(ctx, id, approverID, reason)
}

// Apply delegates to the inner service.
func (o *OverrideService) Apply(ctx context.Context, id, actorID string) (domain.Override, error) {
	return o.inner.Apply(ctx, id, actorID)
}

// EnableForTenant enables the override workflow for a tenant.
func (o *OverrideService) EnableForTenant(ctx context.Context, tenantID string, thresholdYen int64, actorID string) (domain.OverrideThreshold, error) {
	return o.inner.EnableForTenant(ctx, tenantID, thresholdYen, actorID)
}

// ListByTenant returns all overrides for a tenant.
func (o *OverrideService) ListByTenant(ctx context.Context, tenantID string) ([]domain.Override, error) {
	return o.store.ListOverridesByTenant(ctx, tenantID)
}

// ListByTenantAndState returns overrides matching the tenant + state filter.
func (o *OverrideService) ListByTenantAndState(ctx context.Context, tenantID string, state domain.OverrideState) ([]domain.Override, error) {
	return o.store.ListOverridesByTenantAndState(ctx, tenantID, state)
}

// FindActiveOverride returns the currently-applied override for the
// (tenant, sku, channel) triple at `at`, if any.
func (o *OverrideService) FindActiveOverride(ctx context.Context, tenantID, sku, channel string, at time.Time) (domain.Override, bool, error) {
	return o.store.FindActiveOverride(ctx, tenantID, sku, channel, at)
}

// Get returns one override by id.
func (o *OverrideService) Get(ctx context.Context, id string) (domain.Override, error) {
	return o.store.GetOverride(ctx, id)
}

// GetThreshold returns the per-tenant threshold config.
func (o *OverrideService) GetThreshold(ctx context.Context, tenantID string) (domain.OverrideThreshold, error) {
	return o.store.GetThreshold(ctx, tenantID)
}
