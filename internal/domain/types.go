// Package domain contains the pure data types that travel between layers.
// Nothing in this package may import from storage, httpapi, money, etc.
package domain

import "time"

// Tenant represents an isolated pricing tenant.
type Tenant struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// User is a synthetic operator. Roles are coarse-grained: admin, pricing.
type User struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

// Product is a stockable SKU.
type Product struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	SKU      string `json:"sku"`
	Name     string `json:"name"`
	BaseYen  int64  `json:"base_yen"`
}

// Customer is a synthetic buyer.
type Customer struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Segment  string `json:"segment"`
}

// PromotionType enumerates the supported discount shapes.
type PromotionType string

const (
	PromotionPercent PromotionType = "percent"
	PromotionAmount  PromotionType = "amount"
)

// Promotion is a single discount rule that can be applied to a price.
// A promotion can be scoped to a channel ("store", "web", "mobile"), to a
// specific SKU, or to all SKUs in the tenant when SKU is empty.
type Promotion struct {
	ID         string        `json:"id"`
	TenantID   string        `json:"tenant_id"`
	Name       string        `json:"name"`
	Type       PromotionType `json:"type"`
	Value      float64       `json:"value"` // percent 0..100 or amount in JPY
	Channel    string        `json:"channel"`
	SKU        string        `json:"sku,omitempty"` // empty == wildcard
	Priority   int           `json:"priority"`
	ValidFrom  time.Time     `json:"valid_from"`
	ValidUntil time.Time     `json:"valid_until"`
}

// IsActiveAt reports whether the promotion is active at t. The comparison
// is INCLUSIVE on both ends.
func (p Promotion) IsActiveAt(t time.Time) bool {
	return !t.Before(p.ValidFrom) && !t.After(p.ValidUntil)
}

// PricingRequest is the input to /pricing/quote and /pricing/batch.
type PricingRequest struct {
	TenantID   string    `json:"tenant_id"`
	CustomerID string    `json:"customer_id"`
	SKU        string    `json:"sku"`
	Quantity   int64     `json:"quantity"`
	Channel    string    `json:"channel"`
	At         time.Time `json:"at"`
}

// PricingDecision is the output of a pricing evaluation.
type PricingDecision struct {
	RequestHash      string   `json:"request_hash"`
	BaseYen          int64    `json:"base_yen"`
	FinalYen         int64    `json:"final_yen"`
	AppliedIDs       []string `json:"applied_promotion_ids"`
	AuditIDs         []string `json:"audit_ids"`
	ExplanationLines []string `json:"explanation_lines"`
}

// BatchJob is a single batch pricing job.
type BatchJob struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	CreatedAt  time.Time `json:"created_at"`
	TotalRows  int       `json:"total_rows"`
	DoneRows   int       `json:"done_rows"`
	Status     string    `json:"status"`
	ResultHash string    `json:"result_hash,omitempty"`
}

// AuditEvent is a structured record of one pricing-affecting action.
type AuditEvent struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Action    string    `json:"action"`
	Subject   string    `json:"subject"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}

// ConfigVersion is bumped whenever a tenant's configuration changes in a
// way that may affect pricing outcomes.
type ConfigVersion struct {
	TenantID string    `json:"tenant_id"`
	Version  int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}
