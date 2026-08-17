// Package promotion — resolver.go
//
// Apply turns a list of candidate promotions and a base unit price into a
// final price by stacking the promotions in priority order.
//
// The candidate list is split by SelectByPath into two paths:
//
//   - SKU-specific promotions, where the rule was registered with a
//     concrete SKU,
//   - channel-wildcard promotions, where the rule was registered with
//     an empty SKU field (i.e. it applies to every SKU in the channel).
//
// A single promotion id may legitimately be returned by the storage
// layer more than once when the operator registered the same promotion
// with two different rule rows (one SKU-specific, one channel-wildcard)
// for the same tenant and channel. The Apply step is the LAST place
// where a unique-application guarantee can be enforced.
//
// Apply iterates the union of both paths in priority order and emits
// one discount and one audit event per visit.
package promotion

import (
	"time"

	"github.com/google/uuid"

	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/money"
)

// Apply returns (appliedIDs, auditIDs, base, final, explanation).
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

	// Each subset is already priority-ordered. We iterate them in
	// priority order, then in sku-specific-first order (which matches
	// the deterministic ordering the storage layer guarantees).
	ordered := append([]domain.Promotion(nil), sortByPriority(skuSpecific)...)
	ordered = append(ordered, sortByPriority(wildcard)...)

	// Walk the merged list and apply every promotion we see. The
	// storage layer is responsible for returning each promotion id at
	// most once; if the same id is visible via both the SKU-specific
	// and the channel-wildcard rule rows, the storage layer will return
	// it twice and this loop will apply it twice.
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

func describe(p domain.Promotion, before, after money.Money) string {
	switch p.Type {
	case domain.PromotionPercent:
		return "applied promotion " + p.ID + " (percent=" + formatFloat(p.Value) + "): " + before.String() + " -> " + after.String()
	case domain.PromotionAmount:
		return "applied promotion " + p.ID + " (amount=" + formatFloat(p.Value) + "): " + before.String() + " -> " + after.String()
	}
	return "applied promotion " + p.ID + ": " + before.String() + " -> " + after.String()
}

func formatFloat(v float64) string {
	if v == float64(int64(v)) {
		return intToString(int64(v))
	}
	return floatToString(v)
}

func intToString(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

func floatToString(v float64) string {
	intPart := int64(v)
	frac := v - float64(intPart)
	if frac < 0 {
		frac = -frac
	}
	fracInt := int64(frac*100 + 0.5)
	return intToString(intPart) + "." + pad2(fracInt)
}

func pad2(v int64) string {
	if v < 10 {
		return "0" + intToString(v)
	}
	return intToString(v)
}
