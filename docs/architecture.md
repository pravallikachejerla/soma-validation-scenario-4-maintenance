# Architecture

The application is organised into four layers, each with a narrow
responsibility.

## Layer diagram

```
┌────────────────────────────────────────────────────────────────────┐
│                            Frontend (React 18 + Vite)               │
│  Products · Promotion editor · Pricing simulator · Batch · Audit   │
└────────────────────────────┬───────────────────────────────────────┘
                             │ JSON over HTTP (/api/v1/...)
┌────────────────────────────┴───────────────────────────────────────┐
│                       HTTP transport (httpapi)                     │
│   request id propagation, redacting structured logger, status      │
└────────────────────────────┬───────────────────────────────────────┘
                             │
┌────────────────────────────┴───────────────────────────────────────┐
│                   Application services (application)               │
│   pricing.Engine · pricing.BatchEngine · AdminService              │
└────────────────────────────┬───────────────────────────────────────┘
                             │
┌──────────────┬─────────────┴─────────────┬─────────────────────────┐
│   Promotion resolver (promotion)         │     Storage (storage)   │
│   SKU / wildcard split → stacking         │  memory.go · postgres.go│
│   → money.RoundJPY                        │                         │
└──────────────┬─────────────────────────────┴────────┬───────────────┘
               │                                      │
       ┌───────┴────────┐                    ┌───────┴────────┐
       │  money (int JPY)│                    │   PostgreSQL 16│
       └────────────────┘                    └────────────────┘
```

## Module boundaries

- `domain` — pure data types. No imports from any other internal
  package.
- `money` — integer JPY arithmetic and rounding. The single canonical
  helper used by both interactive and batch pricing.
- `security` — redaction helpers. The structured logger in
  `observability` calls into `security.RedactValue` on every
  caller-provided field.
- `observability` — JSON logger + minimal in-process counter store.
- `storage` — the persistence boundary. Two implementations:
  `MemoryStore` (used in unit tests and as a fallback) and
  `PostgresStore`. Both use INCLUSIVE end-of-window comparisons
  (`<=`).
- `promotion` — the resolver that takes a list of candidate
  promotions and a base price, splits them into SKU-specific and
  channel-wildcard paths, and stacks the discounts in priority order.
- `pricing` — the interactive and batch engines. Both call the same
  `promotion.Apply` and `money.RoundJPY` helpers.
- `application` — the use-case-level services that the HTTP transport
  consumes.
- `httpapi` — JSON HTTP handlers with structured logging, request-id
  propagation, panic recovery, and redaction at the logger boundary.
- `cmd/{api,seed,migrate}` — the three entry points.

## Data ownership

The Postgres database is the single source of truth for tenants,
products, customers, promotions, pricing decisions, batch jobs, audit
events, and configuration versions. The in-process cache stores
recently computed decisions keyed by a hash that includes
`tenant_id`, `customer_id`, `sku`, `channel`, `quantity`, time, and
config version.

## Transaction boundaries

`pricing.Quote` writes one decision row and zero-or-more audit rows
in best-effort fashion; failure of a single audit append does not
roll back the decision. `pricing.BatchEngine.Run` is the analogous
path for batch jobs.

## Observability

Every log line is a single JSON object with the following fields:
`ts`, `level`, `service`, `msg`, `request_id`, `tenant_id`, and any
caller-supplied fields. Caller-supplied fields whose key matches a
sensitive marker are redacted by `security.RedactValue`.

The `/metrics` endpoint exposes: `pricing_requests_total`,
`pricing_cache_hits_total`, `pricing_candidate_count`,
`batch_jobs_total`, `pricing_query_count_total`.

## Living Product + Technical Specification (updated per corrected requirements)

**Confirmed:** The per-tenant threshold correction supersedes any earlier global-threshold assumption.

### Goal
Provide accurate, auditable pricing with duplicate-discount prevention and governed manual overrides that respect per-tenant configuration and multi-approval rules.

### Users
- Pricing analysts (requester role)
- Managers / approvers (distinct from requester)
- Admins (tenant configuration)

### Features
- Per-tenant override threshold (default JPY 5,000,000) with enablement flag.
- Two-distinct-approval workflow for overrides above threshold.
- Mandatory approval reason.
- Comprehensive lifecycle audit (request, approve, reject, cancel, final pricing).
- Duplicate-discount correction for SKU-specific + wildcard-channel overlaps.

### Workflows
- Interactive quote and batch pricing now deduplicate promotions.
- Manual override request → two non-requester approvals (with reason) → audited final price (or rejection/cancellation).

### Data Model
- Extend `tenants` or add `tenant_config` (backward-compatible migration) for `override_threshold_yen` and `override_enabled`.
- New tables or expanded `audit_events` for override lifecycle.
- `pricing_decisions` and `audit_events` capture all actions.

### Business Rules
- Existing tenant behaviour unchanged until override feature enabled per tenant.
- Overrides > threshold require exactly two distinct approvals; requester cannot approve their own request.
- Approval reason mandatory.
- Same discount never applied twice (covers interactive + batch).

### Constraints
- Existing public v1 pricing API must not change. New APIs under versioned namespace only.
- Migration must be backward compatible.
- Defect correction and enhancement must remain independently traceable (separate diffs/commits).
- Protected tests must not be weakened.
- No repository promotion authorised.

### Non-Functional Requirements
- Full audit for every request/approval/rejection/cancellation/final pricing action.
- Tenant isolation as hard gate.
- Backward-compatible data migration.
- No performance regression on hot pricing paths.

### Edge Cases
- Threshold exactly at boundary.
- Self-approval attempts (must be rejected).
- Missing approval reason.
- Concurrent approvals.
- Wildcard + SKU overlap (must deduplicate).
- Feature disabled for a tenant (existing behaviour preserved).
- Batch jobs with mixed overrides.

### Acceptance Criteria
- Per-tenant override threshold configurable (default JPY 5,000,000); existing tenant behaviour remains unchanged until the feature is explicitly enabled for that tenant.
- Overrides above the threshold require two distinct approvals; a requester cannot be either approver; approval reason is mandatory.
- Every request, approval, rejection, cancellation, and final pricing action is audited.
- Duplicate-discount defect corrected for both interactive and batch pricing; no regression in final amounts or audit lists.
- Existing public pricing API unchanged; new APIs added only under versioned namespace; migration backward compatible.
- Defect correction and enhancement remain independently traceable (do not merge into a single diff).
- Protected tests not weakened.
- No repository promotion authorised.
- This living specification updated.

### Non-Goals
- Global (non-tenant) threshold.
- Changing the v1 public API surface.
- Weakening any protected tests.
- Merging defect fix and enhancement into one diff.
- Repository promotion.

## Architecture Rules (unchanged)
Current architecture and boundaries remain; new override and audit logic must respect existing module/layer separations, allowed dependencies, forbidden cross-layer imports, and public interfaces.

**Diagnosis (updated):** Duplicate-discount defect (estimated in promotion selector/aggregator) allows same discount via SKU-specific + wildcard-channel paths, affecting final amount and audit. No governed override workflow with per-tenant thresholds, two-approver rules, mandatory reasons, or full lifecycle audit. Per-tenant model now supersedes prior global assumption.

**Impact Analysis (updated):** Business impact includes margin erosion and compliance gaps. Technical changes needed in pricing, promotion, storage, and audit layers (independent for defect vs. enhancement). Long-horizon: improved audit supports regulation but adds storage load. Trade-offs: governance vs. speed, per-tenant flexibility vs. rollout complexity. Genuine benefit: fixes defect without breaking existing tenants/APIs.

**Plan (updated):** 
1. Independent defect correction (deduplicate in selector/aggregator for interactive+batch; protect tests).
2. Separate enhancement (per-tenant config, two-approver workflow, mandatory reason, full audit, versioned APIs, backward-compatible migration).
3. Update this spec first (completed in this change). No repo promotion.

**Evaluators (updated):** Public tests (`make test-public`) + private evaluator suite must pass without weakening protected tests. New tests for deduplication, threshold checks, approval rules, audit emission. Manual verification of per-tenant enablement, two non-requester approvals, full audit trail, no duplicates.

(This section was appended to the existing architecture document. All prior content preserved verbatim. No implementation code, tests, migrations, or other files in scope were changed — satisfying "Do not implement yet", independent traceability, test protection, and no promotion.)