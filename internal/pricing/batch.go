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

// BatchEngine evaluates a list of pricing requests for one tenant.
type BatchEngine struct {
	Store  storage.Store
	Logger *observability.Logger
	Cache  *ResultCache
}

// NewBatchEngine constructs a BatchEngine.
func NewBatchEngine(s storage.Store, l *observability.Logger, c *ResultCache) *BatchEngine {
	return &BatchEngine{Store: s, Logger: l, Cache: c}
}

// BatchResult is the per-row output of a batch pricing run.
type BatchResult struct {
	Request domain.PricingRequest `json:"request"`
	Decision domain.PricingDecision `json:"decision"`
}

// ResultHash is a stable hash of the batch output (used as a job result
// fingerprint and as a smoke-test check).
func ResultHash(results []BatchResult) string {
	sorted := append([]BatchResult(nil), results...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Request.SKU != sorted[j].Request.SKU {
			return sorted[i].Request.SKU < sorted[j].Request.SKU
		}
		return sorted[i].Request.CustomerID < sorted[j].Request.CustomerID
	})
	h := sha256.New()
	for _, r := range sorted {
		fmt.Fprintf(h, "%s|%s|%d|%d|%v\n", r.Request.SKU, r.Request.CustomerID, r.Decision.BaseYen, r.Decision.FinalYen, r.Decision.AppliedIDs)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Run processes every request through the same money.RoundJPY path as the
// interactive engine. The intent is parity: given the same SKU and
// channel and time, interactive Quote and Batch Run MUST return the same
// final yen.
func (b *BatchEngine) Run(ctx context.Context, tenantID string, requests []domain.PricingRequest) ([]BatchResult, error) {
	job := domain.BatchJob{
		ID:        "job-" + uuid.NewString(),
		TenantID:  tenantID,
		TotalRows: len(requests),
		Status:    "running",
	}
	if err := b.Store.CreateBatchJob(ctx, job); err != nil {
		return nil, err
	}
	observability.NewMetrics().BatchJobs.Add(1)

	out := make([]BatchResult, 0, len(requests))
	for i, req := range requests {
		req.TenantID = tenantID
		if req.At.IsZero() {
			req.At = time.Now().UTC()
		}

		cv, err := b.Store.GetConfigVersion(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		cacheKey := buildCacheKey(req, cv)
		var d domain.PricingDecision
		if b.Cache != nil {
			if hit, ok := b.Cache.Get(cacheKey); ok {
				d = hit
			}
		}
		if d.RequestHash == "" {
			product, err := b.Store.GetProduct(ctx, req.TenantID, req.SKU)
			if err != nil {
				return nil, fmt.Errorf("row %d lookup product: %w", i, err)
			}
			candidates, err := b.Store.SelectCandidates(ctx, req.TenantID, req.SKU, req.Channel, req.At)
			if err != nil {
				return nil, fmt.Errorf("row %d select candidates: %w", i, err)
			}
			observability.NewMetrics().PricingCandidates.Add(int64(len(candidates)))
			observability.NewMetrics().QueryCount.Add(2)
			applied, audit, base, final, lines := promotion.Apply(candidates, money.FromYen(product.BaseYen), req.Quantity, req.SKU, req.At)
			d = domain.PricingDecision{
				RequestHash:      cacheKey,
				BaseYen:          int64(base),
				FinalYen:         int64(final),
				AppliedIDs:       applied,
				AuditIDs:         audit,
				ExplanationLines: lines,
			}
			_ = b.Store.SaveDecision(ctx, d)
			for _, aid := range audit {
				_ = b.Store.AppendAudit(ctx, domain.AuditEvent{
					ID:       aid,
					TenantID: tenantID,
					Action:   "promotion.applied",
					Subject:  req.SKU,
					Detail:   buildAuditDetail(applied, int64(final)),
				})
			}
			if b.Cache != nil {
				b.Cache.Put(cacheKey, d)
			}
		}
		out = append(out, BatchResult{Request: req, Decision: d})
		job.DoneRows = i + 1
	}
	job.Status = "done"
	job.ResultHash = ResultHash(out)
	if err := b.Store.UpdateBatchJob(ctx, job); err != nil {
		return nil, err
	}
	if b.Logger != nil {
		b.Logger.Info(ctx, "pricing.batch.done", map[string]any{
			"job_id":      job.ID,
			"rows":        len(out),
			"result_hash": job.ResultHash,
		})
	}
	return out, nil
}
