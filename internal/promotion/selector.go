// Package promotion — selector.go
//
// SelectByPath splits the candidate set into two paths, based on how a
// rule matched the request:
//
//   - SKU-specific path: a rule whose `sku` field is set and matches the
//     request SKU exactly. The same logical promotion can be present
//     here when the operator registered it with a SKU-specific rule.
//
//   - channel-wildcard path: a rule whose `sku` field is empty. This
//     represents a "match any SKU in this channel" rule.
//
// A single promotion can be present in BOTH paths when the operator
// registered the same promotion id with two different rule rows (one
// SKU-specific, one channel-wildcard). The Apply step is responsible
// for any de-duplication.
package promotion

import (
	"sort"

	"github.com/somagen/scenario4/internal/domain"
)

// SelectByPath splits the candidates into two subsets.
func SelectByPath(candidates []domain.Promotion, requestSKU string) (skuSpecific []domain.Promotion, wildcard []domain.Promotion) {
	for _, p := range candidates {
		if p.SKU == "" {
			wildcard = append(wildcard, p)
			continue
		}
		if requestSKU != "" && p.SKU == requestSKU {
			skuSpecific = append(skuSpecific, p)
		}
	}
	return skuSpecific, wildcard
}

// sortByPriority returns a copy of ps sorted by priority DESC then id ASC.
func sortByPriority(ps []domain.Promotion) []domain.Promotion {
	out := append([]domain.Promotion(nil), ps...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out
}
