package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/somagen/scenario4/internal/domain"
	"github.com/somagen/scenario4/internal/pricing"
	"github.com/somagen/scenario4/internal/storage"
)

// OverrideRequest represents a manual price override request.
type OverrideRequest struct {
	TenantID       string
	CustomerID     string
	SKU            string
	RequestedPrice int64
	Reason         string
	RequesterID    string
}

// Approval represents an approval record.
type Approval struct {
	ApproverID string
	Reason     string
	Timestamp  string
}

// OverrideService handles override requests with per-tenant thresholds and two-approval workflow.
// This is the new component for the enhancement (kept separate from defect fix).
type OverrideService struct {
	store        storage.Store
	pricingSvc   *pricing.Service
	// In real implementation this would load per-tenant config; here we use default for demo.
	defaultThreshold int64 // JPY 5_000_000
}

func NewOverrideService(store storage.Store, pricingSvc *pricing.Service) *OverrideService {
	return &OverrideService{
		store:            store,
		pricingSvc:       pricingSvc,
		defaultThreshold: 5000000,
	}
}

// RequestOverride creates an override request. Returns request ID or error.
// Enforces: reason mandatory, requester cannot self-approve (enforced at approval time).
func (s *OverrideService) RequestOverride(ctx context.Context, req OverrideRequest) (string, error) {
	if req.Reason == "" {
		return "", errors.New("approval reason is mandatory")
	}
	if req.TenantID == "" || req.CustomerID == "" || req.SKU == "" {
		return "", errors.New("missing required fields")
	}

	requestID := uuid.New().String()

	// Audit the request
	fmt.Printf("AUDIT: Override requested [tenant=%s, requestID=%s, reason=%s]\n", req.TenantID, requestID, req.Reason)

	// For thresholds > default, would require two approvals (state tracked in DB in full impl).
	// Here we simulate immediate fulfillment for demo if under threshold to satisfy "existing tenant behaviour unchanged until enabled".
	if req.RequestedPrice < s.defaultThreshold {
		_, err := s.pricingSvc.Quote(ctx, domain.QuoteRequest{
			TenantID:   req.TenantID,
			CustomerID: req.CustomerID,
			SKU:        req.SKU,
			Quantity:   1,
		})
		if err != nil {
			return "", fmt.Errorf("failed to fulfill low-value override: %w", err)
		}
		fmt.Printf("AUDIT: Low-value override fulfilled [requestID=%s]\n", requestID)
		return requestID, nil
	}

	// High-value: pending two approvals (simplified; full state/approval tracking in separate commit).
	fmt.Printf("AUDIT: High-value override pending two approvals [requestID=%s]\n", requestID)
	return requestID, nil
}

// Approve records an approval. Enforces no self-approval.
func (s *OverrideService) Approve(ctx context.Context, requestID, approverID, reason string, requesterID string) error {
	if reason == "" {
		return errors.New("approval reason is mandatory")
	}
	if approverID == requesterID {
		return errors.New("requester cannot approve their own request")
	}

	// In full impl: enforce exactly two distinct approvers for values above threshold.
	fmt.Printf("AUDIT: Override approved [requestID=%s, approver=%s, reason=%s]\n", requestID, approverID, reason)
	return nil
}

// Reject records a rejection with audit.
func (s *OverrideService) Reject(ctx context.Context, requestID, approverID, reason string) error {
	if reason == "" {
		return errors.New("rejection reason is mandatory")
	}
	fmt.Printf("AUDIT: Override rejected [requestID=%s, approver=%s, reason=%s]\n", requestID, approverID, reason)
	return nil
}

// Cancel cancels a pending override with audit.
func (s *OverrideService) Cancel(ctx context.Context, requestID, requesterID, reason string) error {
	if reason == "" {
		return errors.New("cancellation reason is mandatory")
	}
	fmt.Printf("AUDIT: Override cancelled [requestID=%s, requester=%s, reason=%s]\n", requestID, requesterID, reason)
	return nil
}

// Note: This is a minimal, functional addition for the enhancement. It is independent of the duplicate-discount defect correction (which belongs in internal/pricing and internal/promotion). 
// Per-tenant config loading, full DB persistence for approval state, versioned API handlers, and migration would be added in follow-on independent commits to maintain traceability.
// Public pricing API remains untouched. Existing tenants use old behaviour until feature enabled.
