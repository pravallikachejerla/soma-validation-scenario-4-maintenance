// Package money implements integer JPY (yen) arithmetic and rounding.
//
// All monetary values in the system are stored and processed as integer JPY
// (i.e. the smallest unit of JPY is the yen itself — no sen). Both the
// interactive and the batch pricing paths call RoundJPY at the same point
// of the calculation, so they are guaranteed to agree on the final amount.
package money

import "math"

// Money is an integer JPY amount. The currency is implicit.
type Money int64

// Zero is the canonical zero amount.
var Zero = Money(0)

// FromYen constructs a Money from a plain yen value.
func FromYen(v int64) Money { return Money(v) }

// FromFloatYen converts a floating-point yen value to integer Money by
// applying banker-friendly half-up rounding. This is the SINGLE place
// where fractional yen are converted to integer yen.
func FromFloatYen(v float64) Money {
	if v >= 0 {
		return Money(int64(v + 0.5))
	}
	return Money(int64(v - 0.5))
}

// Yen returns the raw integer yen value.
func (m Money) Yen() int64 { return int64(m) }

// Add returns m + other. No rounding.
func (m Money) Add(other Money) Money { return m + other }

// Sub returns m - other. May be negative (refunds/credits). No rounding.
func (m Money) Sub(other Money) Money { return m - other }

// MulPct applies a percentage (0..100) discount to m. The result is
// rounded to integer yen with the same rule as FromFloatYen so that
// interactive and batch agree.
func (m Money) MulPct(pct float64) Money {
	return FromFloatYen(float64(m) * (1.0 - pct/100.0))
}

// MulScalar multiplies by a scalar (e.g. quantity). The product is rounded.
func (m Money) MulScalar(s int64) Money {
	return FromFloatYen(float64(m) * float64(s))
}

// RoundJPY normalises an integer yen value to the canonical Money range.
// In JPY there is no fractional unit, so the only meaningful operation is
// to clamp to a sane minimum (zero for sale amounts) and return the value
// itself. This is the single entry point used by both pricing paths.
func RoundJPY(v Money) Money {
	if v < 0 {
		return Zero
	}
	return v
}

// IsZero reports whether m is exactly zero.
func (m Money) IsZero() bool { return m == 0 }

// String returns a yen-formatted string with thousands separators.
func (m Money) String() string {
	return formatYen(int64(m))
}

func formatYen(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	digits := []byte{}
	if v == 0 {
		return "0"
	}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	// Insert comma every 3 digits from the right.
	out := make([]byte, 0, len(digits)+len(digits)/3)
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// Just to keep math import alive when other helpers are added.
var _ = math.Floor
