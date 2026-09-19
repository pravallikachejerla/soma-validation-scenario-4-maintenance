// Package promotion resolves which promotions apply to a pricing request
// and in what order. The package is a pure function — it has no storage
// or I/O. Callers (the application layer) fetch candidates from storage
// and pass them in. Promotion resolution is therefore both deterministic
// and easy to unit-test.
//
// Two storage-layer outputs feed the resolver:
//   - skuSpecific: promotions with SKU == requestSKU (the SKU-specific path)
//   - wildcard:    promotions with SKU == ""      (the channel-wildcard path)
//
// The same promotion id can legitimately appear in BOTH paths when an
// operator registered the promotion with two rule rows: one SKU-specific
// row and one channel-wildcard row. Both rows are active for the request;
// the resolver MUST apply the promotion exactly once.
//
// To enforce that, this package applies an idempotent de-duplication step
// at resolver time: if the same promotion id is present in both paths, the
// SKU-specific rule wins (it is strictly more specific) and the wildcard
// entry is dropped. See the long-form note above dedupAndPreferSKU below.
package promotion

import (
	"time"

	"github.com/google/uuid"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/money"
)

// Apply returns (appliedIDs, auditIDs, base, final, lines).
//
// appliedIDs contains the promotion ids in the order they were applied.
// auditIDs is a synthetic id per application, suitable for storage in
// audit_events.
//
// base is the base unit price in JPY (the price of one unit before any
// discount); final is the unit price after stacking all discounts, as
// produced by money.RoundJPY.
//
// requestSKU is the SKU on the incoming pricing request; it is used to
// decide which candidates belong to the SKU-specific path.
func Apply(candidates []domain.Promotion, base money.Money, quantity int64, requestSKU string, at time.Time) (applied []string, audit []string, b money.Money, f money.Money, lines []string) {
	b = base
	f = base

	skuSpecific, wildcard := SelectByPath(candidates, requestSKU)
	_ = at // at is unused here; storage layer has already time-filtered candidates.

	// De-duplicate by promotion identity (idempotent at-most-once per id).
	//
	// The same promotion id may legitimately be returned twice by the
	// storage layer when the operator registered it with two different
	// rule rows: one SKU-specific and one channel-wildcard. Both rule
	// rows are active for the request, but the discount MUST be applied
	// exactly once.
	//
	// Conflict resolution rule: when the same id is present in both
	// paths, the SKU-specific rule wins. It is strictly more specific
	// (it is constrained to one SKU, not "any SKU in the channel"), so
	// preferring it over the wildcard is the only choice that respects
	// the operator's most precise intent.
	ordered := dedupAndPreferSKU(skuSpecific, wildcard)

	// Walk the merged list and apply every promotion we see. De-dup
	// guarantees we visit each promotion id at most once.
	for _, p := range ordered {
		if !p.IsActiveAt(at) {
			continue
		}
		before := f
		switch p.Type {
		case domain.PromotionPercent:
			f = f.MulPct(p.Value)
		case domain.PromotionAmount:
			f = f.Sub(money.FromFloatYen(p.Value))
		}
		f = money.RoundJPY(f)
		applied = append(applied, p.ID)
		audit = append(audit, "audit-"+uuid.NewString())
		lines = append(lines, describe(p, before, f))
	}
	_ = quantity // quantity is applied by the caller at price-quote time, not here.
	return
}

// dedupAndPreferSKU merges the SKU-specific and wildcard paths into a
// single, priority-ordered list in which each promotion id appears at
// most once. When the same id is present in both paths, the SKU-specific
// rule is kept and the wildcard is dropped.
func dedupAndPreferSKU(skuSpecific, wildcard []domain.Promotion) []domain.Promotion {
	// Each subset is already priority-ordered by the storage layer; we
	// re-sort defensively because callers in tests sometimes construct
	// candidates in unsorted order.
	skuOrdered := sortByPriority(skuSpecific)
	wildOrdered := sortByPriority(wildcard)

	// Build the SKU-specific id set so we can drop the conflicting
	// wildcard entries.
	skuIDs := make(map[string]struct{}, len(skuOrdered))
	for _, p := range skuOrdered {
		skuIDs[p.ID] = struct{}{}
	}
	filteredWildcard := make([]domain.Promotion, 0, len(wildOrdered))
	for _, p := range wildOrdered {
		if _, conflict := skuIDs[p.ID]; conflict {
			// SKU-specific rule already covers this id; wildcard is
			// shadowed.
			continue
		}
		filteredWildcard = append(filteredWildcard, p)
	}

	merged := make([]domain.Promotion, 0, len(skuOrdered)+len(filteredWildcard))
	merged = append(merged, skuOrdered...)
	merged = append(merged, filteredWildcard...)
	return merged
}

// describe renders a one-line human-readable summary of a single
// promotion application, used in audit and explain output.
func describe(p domain.Promotion, before, after money.Money) string {
	switch p.Type {
	case domain.PromotionPercent:
		return p.ID + " " + p.Name + ": " + p.SKU + " " + before.String() + " -> " + after.String()
	case domain.PromotionAmount:
		return p.ID + " " + p.Name + ": " + p.SKU + " " + before.String() + " -off " + after.String()
	}
	return p.ID + ": " + before.String() + " -> " + after.String()
}
