// Package security provides redaction helpers for logs and other surfaces.
//
// The pricing application handles synthetic commercial data only. Logs and
// any other observable text MUST go through these helpers so that
// customer identifiers, negotiated prices and discount reasons never
// appear in clear text.
package security

import (
	"strings"
)

// sensitive keys that are NEVER emitted in clear text in logs.
var sensitiveKeys = map[string]struct{}{
	"customer_id":      {},
	"customer_name":    {},
	"negotiated_price": {},
	"discount_reason":  {},
	"raw_request":      {},
	"raw_response":     {},
	"password":         {},
	"secret":           {},
	"token":            {},
}

// RedactValue returns a redacted placeholder for a sensitive value, or the
// original value if the key is not sensitive. Callers should use this when
// they are unsure whether a value might contain commercial data.
func RedactValue(key string, value string) string {
	if _, ok := sensitiveKeys[strings.ToLower(key)]; ok {
		return "[redacted]"
	}
	return value
}

// RedactMap returns a copy of m with all sensitive values replaced by a
// redacted placeholder. Keys are preserved. This is the recommended
// helper for structured-log fields.
func RedactMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if _, ok := sensitiveKeys[strings.ToLower(k)]; ok {
			out[k] = "[redacted]"
			continue
		}
		switch t := v.(type) {
		case string:
			out[k] = t
		default:
			out[k] = t
		}
	}
	return out
}

// IsSensitiveKey reports whether k is a known sensitive key.
func IsSensitiveKey(k string) bool {
	_, ok := sensitiveKeys[strings.ToLower(k)]
	return ok
}
