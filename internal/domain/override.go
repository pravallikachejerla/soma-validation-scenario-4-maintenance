package domain

import "time"

// OverrideState is the lifecycle state of a manual price-override
// request. The state machine is:
//
//	requested --first-approve--> awaiting_second_approval
//	requested --reject----------> rejected
//	awaiting_second_approval --second-approve--> approved
//	awaiting_second_approval --reject-----------> rejected
//	approved --apply--> applied
//	applied/rejected are terminal.
type OverrideState string

const (
	// OverrideRequested is the initial state, immediately after an
	// operator submits the override request.
	OverrideRequested OverrideState = "requested"

	// OverrideAwaitingSecondApproval is the state after the FIRST
	// distinct approver has approved the request. The request is now
	// waiting for a SECOND distinct approver (distinct from both the
	// requester and the first approver) to either approve or reject.
	OverrideAwaitingSecondApproval OverrideState = "awaiting_second_approval"

	// OverrideApproved is the state after two distinct approvers have
	// both approved the request. The request is now eligible to be
	// applied (the apply step is what actually changes the price).
	OverrideApproved OverrideState = "approved"

	// OverrideRejected is the terminal state for a request that did
	// not receive the required approvals. Rejected requests never
	// reach pricing evaluation.
	OverrideRejected OverrideState = "rejected"

	// OverrideApplied is the terminal state for an approved request
	// whose override price has been written into pricing evaluation.
	// An applied override is the only state that actually affects
	// future pricing decisions for the matched SKU/channel pair.
	OverrideApplied OverrideState = "applied"
)

// OverrideRequesterRole is the coarse role required to request an
// override. Operators without this role cannot create requests.
const OverrideRequesterRole = "pricing"

// OverrideApproverRole is the coarse role required to approve a
// request. Operators with the requester role but not the approver
// role (or vice versa) cannot perform role-separation requirements
// (requester and approver must be different users; first and second
// approver must be different users).
const OverrideApproverRole = "admin"

// Override is a single manual price-override request and its lifecycle.
//
// An override lets an operator pin a manual JPY price for a specific
// (tenant, SKU, channel) triple for a bounded window. The override only
// affects pricing evaluation after it reaches the OverrideApplied state.
//
// Field semantics:
//   - ID: stable per-request identifier (synthesised by the service).
//   - RequesterID: user id that submitted the request.
//   - Approver1ID: user id of the first approver; populated when
//     OverrideState transitions to OverrideAwaitingSecondApproval.
//   - Approver2ID: user id of the second approver; populated when
//     OverrideState transitions to OverrideApproved.
//   - Quantity: quantity the override applies to. The price override
//     is applied on a per-unit basis: an override of 800 JPY for
//     quantity 3 prices all 3 units at 800 JPY each, NOT 2400 JPY
//     total. The quantity is recorded for audit.
//   - ValidFrom / ValidUntil: bounded window during which the override
//     is active once applied. After ValidUntil, the override no longer
//     affects pricing (the state is still OverrideApplied, but the
//     pricing engine ignores expired overrides).
//   - AppliedAt: timestamp at which the override was applied.
//   - Reason: free-text justification provided by the requester.
type Override struct {
	ID           string        `json:"id"`
	TenantID     string        `json:"tenant_id"`
	SKU          string        `json:"sku"`
	Channel      string        `json:"channel"`
	Quantity     int64         `json:"quantity"`
	OverrideYen  int64         `json:"override_yen"`
	RequesterID  string        `json:"requester_id"`
	Approver1ID  string        `json:"approver1_id,omitempty"`
	Approver2ID  string        `json:"approver2_id,omitempty"`
	State        OverrideState `json:"state"`
	Reason       string        `json:"reason"`
	RejectReason string        `json:"reject_reason,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	ValidFrom    time.Time     `json:"valid_from"`
	ValidUntil   time.Time     `json:"valid_until"`
	AppliedAt    time.Time     `json:"applied_at,omitempty"`
}

// IsActiveAt reports whether the override is currently active (its
// state is OverrideApplied AND the current time falls inside the
// valid window). Pricing evaluation MUST call this before using an
// override price.
func (o Override) IsActiveAt(t time.Time) bool {
	if o.State != OverrideApplied {
		return false
	}
	if t.Before(o.ValidFrom) || t.After(o.ValidUntil) {
		return false
	}
	return true
}

// OverrideThreshold is the per-tenant configuration that gates the
// override workflow. While disabled, the override workflow is
// INACTIVE for the tenant — requests return an error and no override
// can be created. When enabled, any override whose OverrideYen is at
// or above ThresholdYen MUST go through the full two-approver
// workflow; overrides below the threshold are rejected.
//
// Default threshold: JPY 5,000,000 (the product spec default).
// Default state:   disabled (inactive until an operator enables it).
type OverrideThreshold struct {
	TenantID     string    `json:"tenant_id"`
	Enabled      bool      `json:"enabled"`
	ThresholdYen int64     `json:"threshold_yen"`
	UpdatedAt    time.Time `json:"updated_at"`
	UpdatedBy    string    `json:"updated_by"`
}

// DefaultOverrideThresholdYen is the product spec default: JPY 5,000,000.
// Override prices at or above this threshold MUST go through two
// approvers; overrides below the threshold are rejected at the gate.
const DefaultOverrideThresholdYen int64 = 5_000_000
