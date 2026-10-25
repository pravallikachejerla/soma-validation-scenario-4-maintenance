// Package domain — override.go
//
// Types for the manual price-override approval workflow.
package domain

import "time"

// DefaultThresholdYen is the default per-tenant threshold above which
// a manual price override requires approval. Each tenant can override
// this via TenantOverrideSettings.ThresholdYen.
const DefaultThresholdYen int64 = 5_000_000

// DefaultRequiredApprovals is the default number of distinct approvers
// required before an override request can be applied.
const DefaultRequiredApprovals = 2

// OverrideStatus is the lifecycle state of an override request.
type OverrideStatus string

const (
	OverridePending  OverrideStatus = "pending"
	OverrideApproved OverrideStatus = "approved"
	OverrideRejected OverrideStatus = "rejected"
	OverrideApplied  OverrideStatus = "applied"
	OverrideExpired  OverrideStatus = "expired"
)

// TenantOverrideSettings holds per-tenant configuration for the
// override workflow.
type TenantOverrideSettings struct {
	TenantID          string `json:"tenant_id"`
	ThresholdYen      int64  `json:"threshold_yen"`
	RequiredApprovals int    `json:"required_approvals"`
	OverrideEnabled   bool   `json:"override_enabled"`
}

// OverrideRequest is a request to apply a manual price override.
type OverrideRequest struct {
	ID            string         `json:"id"`
	TenantID      string         `json:"tenant_id"`
	RequesterID   string         `json:"requester_id"`
	SKU           string         `json:"sku"`
	Channel       string         `json:"channel"`
	BasePriceYen  int64          `json:"base_price_yen"`
	OverrideYen   int64          `json:"override_yen"`
	Reason        string         `json:"reason"`
	Status        OverrideStatus `json:"status"`
	Approvals     []Approval     `json:"approvals"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// Approval is a single approval or rejection record.
type Approval struct {
	ApproverID string    `json:"approver_id"`
	Decision   string    `json:"decision"` // "approved" or "rejected"
	Reason     string    `json:"reason"`
	At         time.Time `json:"at"`
}
