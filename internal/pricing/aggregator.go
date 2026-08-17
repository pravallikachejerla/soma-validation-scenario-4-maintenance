package pricing

import (
	"sync"

	"github.com/somagen/scenario4/internal/domain"
)

// ResultCache is a small in-process cache of pricing decisions, keyed by
// a hash that includes tenant_id, customer, SKU, channel, quantity, time
// and config version. This is the cheap, single-process cache used by the
// API; it is NOT a cross-tenant cache and NOT used in the postgres-only
// deployments where the same key guarantees the same answer anyway.
type ResultCache struct {
	mu      sync.RWMutex
	entries map[string]domain.PricingDecision
	max     int
}

// NewResultCache returns a fresh cache with the given max entries.
func NewResultCache(max int) *ResultCache {
	if max <= 0 {
		max = 256
	}
	return &ResultCache{entries: make(map[string]domain.PricingDecision), max: max}
}

// Get returns a cached decision if present.
func (c *ResultCache) Get(key string) (domain.PricingDecision, bool) {
	if c == nil {
		return domain.PricingDecision{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	d, ok := c.entries[key]
	return d, ok
}

// Put inserts a decision, evicting roughly the oldest quarter if the
// cache is full. Eviction is intentionally simple; this is a synthetic
// benchmark, not a real LRU.
func (c *ResultCache) Put(key string, d domain.PricingDecision) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		// Drop a quarter of the entries to keep things simple.
		drop := c.max / 4
		if drop < 1 {
			drop = 1
		}
		i := 0
		for k := range c.entries {
			if i >= drop {
				break
			}
			delete(c.entries, k)
			i++
		}
	}
	c.entries[key] = d
}

// Size returns the current number of entries (used by metrics).
func (c *ResultCache) Size() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
