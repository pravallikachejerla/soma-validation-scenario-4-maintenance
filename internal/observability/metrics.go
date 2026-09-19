package observability

import (
	"sync/atomic"
)

// Metrics is a tiny atomic counter store. It is enough to surface a
// /metrics endpoint with the application-defined counters without pulling
// in a heavy Prometheus client. The atomic operations are lock-free
// and safe for concurrent pricing requests.
type Metrics struct {
	PricingRequests   atomic.Int64
	PricingCacheHits  atomic.Int64
	PricingCandidates atomic.Int64
	BatchJobs         atomic.Int64
	QueryCount        atomic.Int64
}

// NewMetrics returns a zeroed Metrics.
func NewMetrics() *Metrics { return &Metrics{} }

// Snapshot returns a serialisable map of all counters.
func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"pricing_requests_total":    m.PricingRequests.Load(),
		"pricing_cache_hits_total":  m.PricingCacheHits.Load(),
		"pricing_candidate_count":   m.PricingCandidates.Load(),
		"batch_jobs_total":          m.BatchJobs.Load(),
		"pricing_query_count_total": m.QueryCount.Load(),
	}
}
