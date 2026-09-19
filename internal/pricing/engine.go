// Package pricing contains the interactive and batch pricing engines.
//
// Both the interactive path (POST /pricing/quote) and the batch path
// (POST /pricing/batch) call Resolve on the same in-memory promotion
// resolver and apply the same money.RoundJPY helper, so they MUST agree
// on the final amount for the same input.
package pricing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/money"
	"github.com/somagen/scenario4/internal/observability"
	"github.com/somagen/scenario4/internal/promotion"
	"github.com/somagen/scenario4/internal/storage"
)

// Engine is the interactive pricing engine.
type Engine struct {
	Store  storage.Store
	Logger *observability.Logger
	Cache  *ResultCache
	// OverrideLookup is an optional indirection for retrieving the
	// currently-applied override for a (tenant, sku, channel) triple.
	// When nil, the engine behaves exactly as before — the override
	// workflow becomes a no-op and pricing falls through to the
	// promotion resolver only. Wiring the lookup here, instead of in
	// the pricing package directly, keeps the pricing package
	// independent of the override workflow package.
	OverrideLookup OverrideLookup
}

// OverrideLookup is the minimal interface the pricing engine needs
// from the override subsystem. Defined here so the pricing package
// does not import the override package.
type OverrideLookup interface {
	FindActive(ctx context.Context, tenantID, sku, channel string, at time.Time) (overrideYen int64, ok bool, err error)
}

// NewEngine wires the engine with its dependencies.
func NewEngine(s storage.Store, l *observability.Logger, c *ResultCache) *Engine {
	return &Engine{Store: s, Logger: l, Cache: c}
}

// Quote evaluates a single pricing request and returns the resulting
// decision. The implementation:
//  1. hashes the request to use as a cache key (includes tenant_id);
//  2. reads the base product price;
//  3. asks the resolver for every promotion that applies;
//  4. applies promotions in priority order via the same money.RoundJPY
//     used by the batch path.
func (e *Engine) Quote(ctx context.Context, req domain.PricingRequest) (domain.PricingDecision, error) {
	if req.Quantity <= 0 {
		return domain.PricingDecision{}, fmt.Errorf("quantity must be > 0")
	}
	if req.TenantID == "" || req.SKU == "" || req.Channel == "" {
		return domain.PricingDecision{}, fmt.Errorf("tenant_id, sku and channel are required")
	}
	if req.At.IsZero() {
		req.At = time.Now().UTC()
	}

	cv, err := e.Store.GetConfigVersion(ctx, req.TenantID)
	if err != nil {
		return domain.PricingDecision{}, err
	}

	cacheKey := buildCacheKey(req, cv)
	if e.Cache != nil {
		if hit, ok := e.Cache.Get(cacheKey); ok {
			if e.Logger != nil {
				e.Logger.Info(ctx, "pricing.cache_hit", map[string]any{"key_prefix": cacheKey[:8]})
			}
			observability.NewMetrics().PricingCacheHits.Add(1)
			return hit, nil
		}
	}

	product, err := e.Store.GetProduct(ctx, req.TenantID, req.SKU)
	if err != nil {
		return domain.PricingDecision{}, fmt.Errorf("lookup product: %w", err)
	}

	candidates, err := e.Store.SelectCandidates(ctx, req.TenantID, req.SKU, req.Channel, req.At)
	if err != nil {
		return domain.PricingDecision{}, fmt.Errorf("select candidates: %w", err)
	}

	observability.NewMetrics().PricingCandidates.Add(int64(len(candidates)))
	observability.NewMetrics().PricingRequests.Add(1)
	observability.NewMetrics().QueryCount.Add(2) // product + candidates

	applied, audit, base, final, lines := promotion.Apply(candidates, money.FromYen(product.BaseYen), req.Quantity, req.SKU, req.At)

	// Manual override: when an applied override exists for this
	// (tenant, sku, channel) at this time, it pins the JPY unit
	// price and the override is recorded in the decision. The
	// override is checked AFTER the promotion resolver so the audit
	// trail still shows the promotion-derived price that was
	// overridden. Per-line totals and the AppliedIDs list are
	// updated to reflect the override.
	overrideApplied := ""
	if e.OverrideLookup != nil {
		ovYen, ok, err := e.OverrideLookup.FindActive(ctx, req.TenantID, req.SKU, req.Channel, req.At)
		if err != nil {
			return domain.PricingDecision{}, fmt.Errorf("lookup override: %w", err)
		}
		if ok {
			overrideApplied = "manual-override"
			base = money.FromYen(ovYen)
			final = money.FromYen(ovYen).MulScalar(req.Quantity)
			lines = append(lines, fmt.Sprintf("override applied: unit=%d qty=%d total=%d", ovYen, req.Quantity, int64(final)))
			applied = append(applied, overrideApplied)
		}
	}

	d := domain.PricingDecision{
		RequestHash:      cacheKey,
		BaseYen:          int64(base),
		FinalYen:         int64(final),
		AppliedIDs:       applied,
		AuditIDs:         audit,
		ExplanationLines: lines,
	}
	_ = e.Store.SaveDecision(ctx, d)
	for _, aid := range audit {
		_ = e.Store.AppendAudit(ctx, domain.AuditEvent{
			ID:       aid,
			TenantID: req.TenantID,
			Action:   "promotion.applied",
			Subject:  req.SKU,
			Detail:   buildAuditDetail(applied, int64(final)),
		})
	}
	if e.Cache != nil {
		e.Cache.Put(cacheKey, d)
	}
	if e.Logger != nil {
		e.Logger.Info(ctx, "pricing.evaluated", map[string]any{
			"sku":          req.SKU,
			"qty":          req.Quantity,
			"candidate_ct": len(candidates),
			"final_yen":    int64(final),
		})
	}
	return d, nil
}

func buildCacheKey(req domain.PricingRequest, configVersion int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d",
		req.TenantID, req.CustomerID, req.SKU, req.Channel, req.Quantity, req.At.UTC().UnixNano(), configVersion)))
	return hex.EncodeToString(sum[:])
}

func buildAuditDetail(applied []string, final int64) string {
	if len(applied) == 0 {
		return fmt.Sprintf("no promotions applied; final=%d", final)
	}
	sorted := append([]string(nil), applied...)
	sort.Strings(sorted)
	return fmt.Sprintf("applied=%v; final=%d", sorted, final)
}

// NewAuditID is exported for the batch path.
func NewAuditID() string { return "audit-" + uuid.NewString() }
