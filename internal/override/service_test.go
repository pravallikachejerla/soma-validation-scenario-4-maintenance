// Package override implements the manual price-override approval workflow.
//
// A manual override lets an operator pin a JPY unit price for a
// specific (tenant, SKU, channel) triple for a bounded window, but only
// after it has cleared a two-distinct-approver review with role
// separation (requester ≠ approver, approver1 ≠ approver2). The override
// is gated by a per-tenant threshold: while disabled, the workflow is
// inactive; when enabled, only overrides whose OverrideYen is at or
// above the configured ThresholdYen may be requested.
//
// The override package exposes a pure service (no I/O) that operates
// on a storage.OverrideRepo. The service performs:
//
//   - request creation (state -> requested)
//   - first approval (state -> awaiting_second_approval) with the
//     constraint that the first approver is distinct from the requester
//   - second approval (state -> approved) with the constraint that the
//     second approver is distinct from both the requester and the first
//     approver
//   - rejection (any non-terminal state -> rejected)
//   - apply (approved -> applied; this is what activates the override)
//
// Every transition writes an audit entry and bumps the tenant config
// version so cached pricing decisions are invalidated.
//
// This file is the failing-test-first specification. Each test names
// the rule it is asserting; the implementation lives in service.go.
package override

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/somagen/scenario4/internal/domain"
)

// memRepo is an in-memory OverrideRepo + OverrideThresholdRepo used
// only by the tests in this file. It records every AppendAudit call so
// tests can assert on audit-emission invariants.
type memRepo struct {
	overrides     map[string]domain.Override
	thresholds    map[string]domain.OverrideThreshold
	audits        []domain.AuditEvent
	configVersion map[string]int64
}

func newMemRepo() *memRepo {
	return &memRepo{
		overrides:     map[string]domain.Override{},
		thresholds:    map[string]domain.OverrideThreshold{},
		configVersion: map[string]int64{},
	}
}

// CreateOverride inserts a new override.
func (m *memRepo) CreateOverride(_ context.Context, o domain.Override) error {
	if _, dup := m.overrides[o.ID]; dup {
		return errors.New("duplicate id")
	}
	m.overrides[o.ID] = o
	return nil
}

// GetOverride returns an override by id.
func (m *memRepo) GetOverride(_ context.Context, id string) (domain.Override, error) {
	o, ok := m.overrides[id]
	if !ok {
		return domain.Override{}, ErrNotFound
	}
	return o, nil
}

// UpdateOverride replaces an existing override.
func (m *memRepo) UpdateOverride(_ context.Context, o domain.Override) error {
	if _, ok := m.overrides[o.ID]; !ok {
		return ErrNotFound
	}
	m.overrides[o.ID] = o
	return nil
}

// ListOverridesByTenant returns all overrides for a tenant, sorted
// by CreatedAt descending.
func (m *memRepo) ListOverridesByTenant(_ context.Context, tenantID string) ([]domain.Override, error) {
	out := []domain.Override{}
	for _, o := range m.overrides {
		if o.TenantID == tenantID {
			out = append(out, o)
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// GetThreshold returns the per-tenant override threshold configuration.
func (m *memRepo) GetThreshold(_ context.Context, tenantID string) (domain.OverrideThreshold, error) {
	t, ok := m.thresholds[tenantID]
	if !ok {
		return domain.OverrideThreshold{TenantID: tenantID, Enabled: false, ThresholdYen: domain.DefaultOverrideThresholdYen}, nil
	}
	return t, nil
}

// UpsertThreshold inserts or updates a per-tenant override threshold.
func (m *memRepo) UpsertThreshold(_ context.Context, t domain.OverrideThreshold) error {
	m.thresholds[t.TenantID] = t
	return nil
}

// AppendAudit records an audit event.
func (m *memRepo) AppendAudit(_ context.Context, e domain.AuditEvent) error {
	m.audits = append(m.audits, e)
	return nil
}

// BumpConfigVersion increments the tenant config version.
func (m *memRepo) BumpConfigVersion(_ context.Context, tenantID string) (int64, error) {
	m.configVersion[tenantID]++
	return m.configVersion[tenantID], nil
}

// ErrNotFound is returned when an override or threshold does not exist.
var ErrNotFound = errors.New("not found")

// fixedClock returns a clock pinned to a specific time.
func fixedClock() time.Time {
	return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
}

// TestRequest_WhenThresholdDisabled_IsRejected asserts the inactive-until-
// enabled rule: when the per-tenant threshold config is disabled
// (the default), the request step returns ErrOverrideNotEnabled and
// no override is created.
func TestRequest_WhenThresholdDisabled_IsRejected(t *testing.T) {
	repo := newMemRepo()
	svc := NewService(repo, fixedClock)
	_, err := svc.Request(context.Background(), RequestInput{
		TenantID:    "tenant-a",
		SKU:         "SKU-00001",
		Channel:     "web",
		Quantity:    1,
		OverrideYen: 800,
		RequesterID: "u-pricing-1",
		Reason:      "match competitor",
	})
	if !errors.Is(err, ErrOverrideNotEnabled) {
		t.Fatalf("expected ErrOverrideNotEnabled, got %v", err)
	}
}

// TestRequest_WhenBelowThreshold_IsRejected asserts the threshold rule:
// when the override amount is below the configured threshold, the
// request is rejected with ErrBelowThreshold (NOT silently applied,
// NOT auto-approved).
func TestRequest_WhenBelowThreshold_IsRejected(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	svc := NewService(repo, fixedClock)
	_, err := svc.Request(context.Background(), RequestInput{
		TenantID:    "tenant-a",
		SKU:         "SKU-00001",
		Channel:     "web",
		Quantity:    1,
		OverrideYen: 100, // far below 5,000,000 threshold
		RequesterID: "u-pricing-1",
		Reason:      "test",
	})
	if !errors.Is(err, ErrBelowThreshold) {
		t.Fatalf("expected ErrBelowThreshold, got %v", err)
	}
}

// TestRequest_WhenAtThreshold_IsAccepted asserts the inclusive boundary:
// the threshold rule applies at-or-above, so an override exactly equal
// to the threshold is accepted into the workflow.
func TestRequest_WhenAtThreshold_IsAccepted(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	svc := NewService(repo, fixedClock)
	o, err := svc.Request(context.Background(), RequestInput{
		TenantID:    "tenant-a",
		SKU:         "SKU-00001",
		Channel:     "web",
		Quantity:    1,
		OverrideYen: 5_000_000, // exactly at threshold
		RequesterID: "u-pricing-1",
		Reason:      "exactly-at-threshold",
	})
	if err != nil {
		t.Fatalf("expected accepted, got %v", err)
	}
	if o.State != domain.OverrideRequested {
		t.Fatalf("expected state=requested, got %s", o.State)
	}
	if o.Approver1ID != "" || o.Approver2ID != "" {
		t.Fatalf("expected no approvers populated at request time, got %q %q", o.Approver1ID, o.Approver2ID)
	}
}

// TestApprove_FirstApproverRejectsSelfApproval asserts role separation
// step 1: the requester CANNOT be the first approver. The service
// returns ErrRoleSeparation.
func TestApprove_FirstApproverRejectsSelfApproval(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	svc := NewService(repo, fixedClock)
	o, err := svc.Request(context.Background(), RequestInput{
		TenantID: "tenant-a", SKU: "SKU-00001", Channel: "web",
		Quantity: 1, OverrideYen: 5_000_000,
		RequesterID: "u-pricing-1", Reason: "test",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	_, err = svc.FirstApprove(context.Background(), o.ID, "u-pricing-1")
	if !errors.Is(err, ErrRoleSeparation) {
		t.Fatalf("expected ErrRoleSeparation, got %v", err)
	}
}

// TestApprove_FirstApproveThenSecondApproveCannotBeFirstApprover asserts
// role separation step 2: the second approver CANNOT be the same user
// as the first approver.
func TestApprove_FirstApproveThenSecondApproveCannotBeFirstApprover(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	svc := NewService(repo, fixedClock)
	o, err := svc.Request(context.Background(), RequestInput{
		TenantID: "tenant-a", SKU: "SKU-00001", Channel: "web",
		Quantity: 1, OverrideYen: 5_000_000,
		RequesterID: "u-pricing-1", Reason: "test",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.FirstApprove(context.Background(), o.ID, "u-admin-1"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	_, err = svc.SecondApprove(context.Background(), o.ID, "u-admin-1")
	if !errors.Is(err, ErrRoleSeparation) {
		t.Fatalf("expected ErrRoleSeparation, got %v", err)
	}
}

// TestApprove_FirstApproveThenSecondApproveCannotBeRequester asserts
// role separation step 3: the second approver CANNOT be the requester.
func TestApprove_FirstApproveThenSecondApproveCannotBeRequester(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	svc := NewService(repo, fixedClock)
	o, err := svc.Request(context.Background(), RequestInput{
		TenantID: "tenant-a", SKU: "SKU-00001", Channel: "web",
		Quantity: 1, OverrideYen: 5_000_000,
		RequesterID: "u-pricing-1", Reason: "test",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := svc.FirstApprove(context.Background(), o.ID, "u-admin-1"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	_, err = svc.SecondApprove(context.Background(), o.ID, "u-pricing-1")
	if !errors.Is(err, ErrRoleSeparation) {
		t.Fatalf("expected ErrRoleSeparation, got %v", err)
	}
}

// TestApprove_RequesterCannotApproveOwnRequest is the explicit, named
// regression for the self-approval prohibition: an operator who created
// an override request must never be allowed to approve it themselves,
// regardless of which approval slot they try to fill. This is exactly
// the case the workflow is designed to prevent — a single user
// proposing AND ratifying a price change without independent review.
//
// The override service enforces this in two places:
//   - FirstApprove refuses the transition when approverID == requesterID
//     (a requester can never cast the first approval vote).
//   - SecondApprove refuses the transition when approverID ==
//     requesterID OR approverID == first approverID (a requester can
//     never cast the second approval vote either, even after a distinct
//     first approver has signed off).
//
// Both paths must be denied with ErrRoleSeparation. Neither transition
// must mutate the override's state, write an audit event, or bump the
// tenant config version.
func TestApprove_RequesterCannotApproveOwnRequest(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	clock := fixedClock()
	svc := NewService(repo, func() time.Time { return clock })
	const requester = "u-pricing-1"
	o, err := svc.Request(context.Background(), RequestInput{
		TenantID: "tenant-a", SKU: "SKU-00001", Channel: "web",
		Quantity: 1, OverrideYen: 5_500_000,
		RequesterID: requester, Reason: "match market",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	// Snapshot pre-attack state so we can prove the failed approval
	// attempts did not mutate the override or write an audit row.
	// (The legitimate first approve below is expected to write one
	// audit row; we snapshot AFTER it so we are measuring only the
	// self-approval attempts.)
	preAudits := -1
	preVersion := int64(-1)

	// Path 1: requester tries to be the FIRST approver — must be denied.
	_, err = svc.FirstApprove(context.Background(), o.ID, requester)
	if !errors.Is(err, ErrRoleSeparation) {
		t.Fatalf("first approve by requester: expected ErrRoleSeparation, got %v", err)
	}

	// State must still be requested; no approver slots populated.
	after1, err := svc.Repo.GetOverride(context.Background(), o.ID)
	if err != nil {
		t.Fatalf("get override after first-attempt: %v", err)
	}
	if after1.State != domain.OverrideRequested {
		t.Fatalf("after denied first-approve, want state=requested, got %s", after1.State)
	}
	if after1.Approver1ID != "" || after1.Approver2ID != "" {
		t.Fatalf("after denied first-approve, want no approvers populated, got approver1=%q approver2=%q",
			after1.Approver1ID, after1.Approver2ID)
	}

	// A different, distinct approver signs off legitimately so the
	// override is in awaiting_second_approval, the state from which
	// the requester could try to cast the SECOND approval vote.
	if _, err := svc.FirstApprove(context.Background(), o.ID, "u-admin-1"); err != nil {
		t.Fatalf("first approve by distinct admin: %v", err)
	}

	// Snapshot NOW — AFTER the legitimate first approve — so we are
	// only measuring side effects of the requester's subsequent
	// second-approval attempt, which must be denied.
	preAudits = len(repo.audits)
	preVersion = repo.configVersion["tenant-a"]

	// Path 2: requester tries to be the SECOND approver — must ALSO
	// be denied (the original requester is barred from every
	// approval slot in the workflow, not just the first).
	_, err = svc.SecondApprove(context.Background(), o.ID, requester)
	if !errors.Is(err, ErrRoleSeparation) {
		t.Fatalf("second approve by requester: expected ErrRoleSeparation, got %v", err)
	}

	after2, err := svc.Repo.GetOverride(context.Background(), o.ID)
	if err != nil {
		t.Fatalf("get override after second-attempt: %v", err)
	}
	if after2.State != domain.OverrideAwaitingSecondApproval {
		t.Fatalf("after denied second-approve, want state=awaiting_second_approval, got %s", after2.State)
	}
	if after2.Approver2ID != "" {
		t.Fatalf("after denied second-approve, want approver2 empty, got %q", after2.Approver2ID)
	}

	// The denied second-approval attempt must not have written an
	// audit row or bumped the tenant config version — those side
	// effects only fire on a SUCCESSFUL transition.
	if len(repo.audits) != preAudits {
		t.Fatalf("denied self-approval wrote audit events: pre=%d post=%d", preAudits, len(repo.audits))
	}
	if repo.configVersion["tenant-a"] != preVersion {
		t.Fatalf("denied self-approval bumped config version: pre=%d post=%d",
			preVersion, repo.configVersion["tenant-a"])
	}
}

// TestApply_HappyPath_TwoDistinctApprovers walks the full happy path:
// request -> first approve -> second approve -> apply. After apply,
// the override is in OverrideApplied state and IsActiveAt returns true.
func TestApply_HappyPath_TwoDistinctApprovers(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	clock := fixedClock()
	svc := NewService(repo, func() time.Time { return clock })
	o, err := svc.Request(context.Background(), RequestInput{
		TenantID: "tenant-a", SKU: "SKU-00001", Channel: "web",
		Quantity: 1, OverrideYen: 5_500_000,
		RequesterID: "u-pricing-1", Reason: "match market",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	o2, err := svc.FirstApprove(context.Background(), o.ID, "u-admin-1")
	if err != nil {
		t.Fatalf("first approve: %v", err)
	}
	if o2.State != domain.OverrideAwaitingSecondApproval {
		t.Fatalf("after first approve, want awaiting_second_approval, got %s", o2.State)
	}
	if o2.Approver1ID != "u-admin-1" {
		t.Fatalf("approver1 not set: %q", o2.Approver1ID)
	}
	o3, err := svc.SecondApprove(context.Background(), o.ID, "u-admin-2")
	if err != nil {
		t.Fatalf("second approve: %v", err)
	}
	if o3.State != domain.OverrideApproved {
		t.Fatalf("after second approve, want approved, got %s", o3.State)
	}
	if o3.Approver2ID != "u-admin-2" {
		t.Fatalf("approver2 not set: %q", o3.Approver2ID)
	}
	clock = clock.Add(10 * time.Minute)
	o4, err := svc.Apply(context.Background(), o.ID, "u-admin-1")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if o4.State != domain.OverrideApplied {
		t.Fatalf("after apply, want applied, got %s", o4.State)
	}
	if !o4.IsActiveAt(clock) {
		t.Fatalf("applied override should be active at %v", clock)
	}
}

// TestApply_BeforeSecondApproval_IsRejected asserts the ordering
// invariant: apply cannot run before two-distinct-approver review has
// completed.
func TestApply_BeforeSecondApproval_IsRejected(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	svc := NewService(repo, fixedClock)
	o, _ := svc.Request(context.Background(), RequestInput{
		TenantID: "tenant-a", SKU: "SKU-00001", Channel: "web",
		Quantity: 1, OverrideYen: 5_500_000,
		RequesterID: "u-pricing-1", Reason: "test",
	})
	if _, err := svc.FirstApprove(context.Background(), o.ID, "u-admin-1"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	_, err := svc.Apply(context.Background(), o.ID, "u-admin-1")
	if !errors.Is(err, ErrNotApproved) {
		t.Fatalf("expected ErrNotApproved, got %v", err)
	}
}

// TestReject_AnyNonTerminalState asserts rejection is allowed at any
// non-terminal state. A reject after first-approve transitions to
// rejected (terminal).
func TestReject_AnyNonTerminalState(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	svc := NewService(repo, fixedClock)
	o, _ := svc.Request(context.Background(), RequestInput{
		TenantID: "tenant-a", SKU: "SKU-00001", Channel: "web",
		Quantity: 1, OverrideYen: 5_500_000,
		RequesterID: "u-pricing-1", Reason: "test",
	})
	if _, err := svc.FirstApprove(context.Background(), o.ID, "u-admin-1"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	o2, err := svc.Reject(context.Background(), o.ID, "u-admin-1", "wrong reason")
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if o2.State != domain.OverrideRejected {
		t.Fatalf("want rejected, got %s", o2.State)
	}
	if !strings.Contains(o2.RejectReason, "wrong reason") {
		t.Fatalf("reject reason not recorded: %q", o2.RejectReason)
	}
}

// TestReject_OfApplied_IsRejected asserts terminal-state immutability:
// once an override is OverrideApplied, the reject transition is
// refused (ErrAlreadyTerminal).
func TestReject_OfApplied_IsRejected(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	clock := fixedClock()
	svc := NewService(repo, func() time.Time { return clock })
	o, _ := svc.Request(context.Background(), RequestInput{
		TenantID: "tenant-a", SKU: "SKU-00001", Channel: "web",
		Quantity: 1, OverrideYen: 5_500_000,
		RequesterID: "u-pricing-1", Reason: "test",
	})
	if _, err := svc.FirstApprove(context.Background(), o.ID, "u-admin-1"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	if _, err := svc.SecondApprove(context.Background(), o.ID, "u-admin-2"); err != nil {
		t.Fatalf("second approve: %v", err)
	}
	clock = clock.Add(time.Minute)
	if _, err := svc.Apply(context.Background(), o.ID, "u-admin-1"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	_, err := svc.Reject(context.Background(), o.ID, "u-admin-1", "too late")
	if !errors.Is(err, ErrAlreadyTerminal) {
		t.Fatalf("expected ErrAlreadyTerminal, got %v", err)
	}
}

// TestAudit_IsWrittenForEveryTransition asserts the audit invariant:
// every state transition writes an audit event with the correct
// action string. We count audit events at the end of the happy path
// and verify each is present.
func TestAudit_IsWrittenForEveryTransition(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	clock := fixedClock()
	svc := NewService(repo, func() time.Time { return clock })
	o, _ := svc.Request(context.Background(), RequestInput{
		TenantID: "tenant-a", SKU: "SKU-00001", Channel: "web",
		Quantity: 1, OverrideYen: 5_500_000,
		RequesterID: "u-pricing-1", Reason: "test",
	})
	if _, err := svc.FirstApprove(context.Background(), o.ID, "u-admin-1"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	if _, err := svc.SecondApprove(context.Background(), o.ID, "u-admin-2"); err != nil {
		t.Fatalf("second approve: %v", err)
	}
	clock = clock.Add(time.Minute)
	if _, err := svc.Apply(context.Background(), o.ID, "u-admin-1"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	wantActions := []string{
		"override.requested",
		"override.first_approved",
		"override.second_approved",
		"override.applied",
	}
	got := map[string]bool{}
	for _, e := range repo.audits {
		got[e.Action] = true
	}
	for _, want := range wantActions {
		if !got[want] {
			t.Fatalf("expected audit action %q, only got %v", want, got)
		}
	}
}

// TestConfigVersion_BumpedOnApply asserts that pricing cache invalidation
// is wired in: when an override is applied, the tenant config version
// is bumped (so cached pricing decisions for that tenant are
// invalidated).
func TestConfigVersion_BumpedOnApply(t *testing.T) {
	repo := newMemRepo()
	repo.thresholds["tenant-a"] = domain.OverrideThreshold{
		TenantID: "tenant-a", Enabled: true,
		ThresholdYen: 5_000_000, UpdatedAt: fixedClock(),
	}
	clock := fixedClock()
	svc := NewService(repo, func() time.Time { return clock })
	o, _ := svc.Request(context.Background(), RequestInput{
		TenantID: "tenant-a", SKU: "SKU-00001", Channel: "web",
		Quantity: 1, OverrideYen: 5_500_000,
		RequesterID: "u-pricing-1", Reason: "test",
	})
	if _, err := svc.FirstApprove(context.Background(), o.ID, "u-admin-1"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	if _, err := svc.SecondApprove(context.Background(), o.ID, "u-admin-2"); err != nil {
		t.Fatalf("second approve: %v", err)
	}
	clock = clock.Add(time.Minute)
	if _, err := svc.Apply(context.Background(), o.ID, "u-admin-1"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if repo.configVersion["tenant-a"] < 1 {
		t.Fatalf("expected config version bumped on apply, got %d", repo.configVersion["tenant-a"])
	}
}
