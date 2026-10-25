# SOMA Pricing — Scenario 4 Analysis Report (REVISION 2)

**Session:** `91c11142-0cbc-4cb1-b39e-287031ef2dfe`
**Turn:** `61716fe3-8bc2-476c-a17d-91b15013e708`
**Repository:** https://github.com/pravallikachejerla/soma-validation-scenario-4-maintenance
**Stack (unchanged):** Go 1.22+ API · React/Vite frontend · PostgreSQL 15
**Status:** Analysis only — no source changes applied. No repository promotion authorised.

---

## 0. Revision 2 — what changed and why

This revision incorporates the **corrected requirements** delivered in the latest user message. The previous draft (Revision 1) made assumptions that the user has now overridden:

| # | Revision 1 assumption | Corrected requirement (this revision) |
|---|---|---|
| 1 | Threshold default `0` (override workflow disabled by default) | **Default threshold is JPY 5,000,000** per tenant |
| 2 | Single approval above threshold | **Two approvals** required above threshold |
| 3 | Threshold semantics were an open question | **Per-tenant, configurable, default JPY 5,000,000** |
| 4 | Feature flag was a recommendation | **Existing tenant behaviour must remain unchanged until the feature is enabled** — feature flag is mandatory, not optional |
| 5 | Defect and enhancement were discussed together | **Defect correction and enhancement must remain independently traceable** — separate commits, separate migrations, separate test files |
| 6 | Public pricing API change was a question | **The existing public pricing API must not change** — overrides are additive only |
| 7 | Migration shape was a question | **The migration must be backward compatible** — additive only, no destructive ALTERs |
| 8 | Test surface was implicit | **Protected tests must not be weakened** — `private/condition_test.go`, `private/clean_baseline_test.go`, `tests/public/*` are protected |
| 9 | Promotion was implicit | **No repository promotion is authorised** — no `git push`, no PR, no merge |
| 10 | Self-approval was a recommendation | **A requester cannot approve their own request** — hard requirement |
| 11 | Reason field was a recommendation | **An approval reason is mandatory** — hard requirement |
| 12 | Audit was a recommendation | **Every request, approval, rejection and final pricing action is audited** — hard requirement |
| 13 | Defect scope was implicit | **The duplicate-discount correction must cover interactive and batch pricing** — both paths must be fixed |

**Explicit confirmation (per the user's instruction):** The **tenant-specific threshold** (per-tenant, configurable, default JPY 5,000,000) **supersedes any earlier global-threshold assumption** made in Revision 1. There is no global threshold. There is no environment-wide default. The threshold is a per-tenant value stored in `config_versions` (or a new `tenant_settings` table — see §8.2), and the default for any tenant that has not been explicitly configured is JPY 5,000,000.

---

## 1. Baseline deployment

| Component | Address | Notes |
|---|---|---|
| API | `http://localhost:8080` | `go run ./cmd/api`, healthz returns `{"status":"ok"}` |
| Frontend | `http://localhost:5173` | `npm run preview` against `dist/` |
| Database | `postgres://pricing:pricing@/pricing` (socket `/run/postgresql`) | `initdb` + `cmd/migrate` + `cmd/seed` |

Build commands actually executed in this session:

```
go build ./...                                          # OK
go run ./cmd/migrate -direction up                      # OK
go run ./cmd/seed                                       # OK
DATABASE_URL=... LISTEN_ADDR=:8080 go run ./cmd/api     # OK
cd frontend && npm install && npm run build             # OK
cd frontend && npm run preview                         # OK
```

The frontend is a static SPA; the API is a long-running Go process. Both are reachable from the sandbox. The frontend is the user-facing surface; the API is the system of record for pricing decisions.

**Baseline URL (sandbox-internal, not public):** `http://localhost:8080` (API) and `http://localhost:5173` (frontend). The frontend is static and could be published to a permanent public URL on request; the Go API and PostgreSQL cannot be hosted here, they only run inside this environment.

---

## 2. Architecture, pricing flow, data model, tests

### 2.1 Architecture (from `docs/architecture.md` and source)

- **Backend** (`internal/`): layered Go service.
  - `domain/` — value types (`Promotion`, `Product`, `PricingDecision`, `BatchJob`, `User`).
  - `storage/` — `PostgresStore` (production) and `MemoryStore` (tests). Both implement the same `Store` interface.
  - `promotion/` — `selector.go` splits candidates by path; `resolver.go` stacks discounts.
  - `pricing/` — `engine.go` orchestrates a quote; `aggregator.go` aggregates line items; `batch.go` handles bulk jobs.
  - `httpapi/` — Gin handlers, request validation, structured logging.
  - `application/` — use-case orchestration (quote, override, batch).
  - `money/` — integer JPY arithmetic (no floats in the hot path).
  - `security/` — auth middleware and redaction.
  - `observability/` — request IDs, metrics.
- **Frontend** (`frontend/src/`): React + Vite SPA. Calls the API directly.
- **Database** (`migrations/0001_init.sql`): `tenants`, `users`, `products`, `promotions`, `pricing_decisions`, `audit_events`, `batch_jobs`, `config_versions`.

### 2.2 Pricing flow (interactive quote)

1. Client `POST /api/v1/pricing/quote` with `{tenant_id, sku, channel, customer_id, quantity, at}`.
2. Handler validates, calls `application.Quote`.
3. `engine.go` loads the product's `base_yen`, then calls `storage.SelectCandidates(tenant, sku, channel, at)`.
4. `promotion.Apply` splits candidates via `SelectByPath` into SKU-specific and channel-wildcard subsets, sorts each by priority DESC then id ASC, concatenates, and walks the merged list applying each promotion in order.
5. Result is persisted to `pricing_decisions` (idempotent on `request_hash`) and `audit_events`.

### 2.3 Data model (key tables)

- `promotions(id, tenant_id, name, type, value, channel, sku, priority, valid_from, valid_until)` — `sku=''` means channel-wildcard.
- `products(id, tenant_id, sku, name, base_yen)`.
- `pricing_decisions(request_hash PK, base_yen, final_yen, applied_ids[], audit_ids[], explanation[], created_at)`.
- `audit_events(id, request_hash, kind, payload, created_at)`.
- `config_versions(tenant_id PK, version)` — currently used as a monotonic version counter, not as a key/value store.

### 2.4 Tests

- `private/condition_test.go` — scenario condition (defect detection). **PROTECTED.**
- `private/clean_baseline_test.go` — clean-baseline guard. **PROTECTED.**
- `internal/promotion/duplicate_discount_test.go` — pins the defect (added in Revision 1). **PROTECTED.**
- `tests/public/pricing_test.go`, `tests/public/batch_test.go`, `tests/public/promotion_test.go`, `tests/public/health_test.go` — public engineering checks. **PROTECTED.**

---

## 3. Defect reproduction

**Defect statement:** Some customers receive the same discount twice when a promotion qualifies through both an SKU-specific path and a wildcard channel path.

**Reproduction (live, against the running baseline):**

```
$ curl -s -X POST http://localhost:8080/api/v1/pricing/quote \
    -H 'Content-Type: application/json' \
    -d '{"tenant_id":"tenant-a","sku":"SKU-00020","channel":"web","customer_id":"cust-001","quantity":1,"at":"2026-06-15T12:00:00Z"}'
```

The response includes `applied_promotion_ids` containing `promo-0004` **twice** (once via the SKU-specific rule row, once via the channel-wildcard rule row). The final price is therefore lower than the operator intended.

**Pinned regression test:** `internal/promotion/duplicate_discount_test.go::TestDuplicateDiscount_DualPathPromotion` — fails on the unchanged source with:

```
duplicate-discount defect: promo-0004 applied 2 times, want exactly 1
(applied=[promo-0004 promo-0004], final=3,068)
```

The two control tests (`OnlyWildcardMatches`, `DistinctIDsUnaffected`) pass, confirming the test isolates the dual-path case.

**Scenario evaluator test:** `private/condition_test.go::TestCondition_MAINT_DEF_01` and `TestCondition_MAINT_DEF_01_ResolverDirect` — both fail on the unchanged source with `expected exactly 1 application, got 2` and `expected 900 yen, got 810 yen`. **These tests are protected and must turn green after the fix.**

---

## 4. Supported root cause and affected paths

**Root cause (supported by source):**

- `internal/storage/postgres.go::SelectCandidates` (lines 227–252) returns **every** row from `promotions` matching `(tenant_id, channel, valid_from <= at <= valid_until)`. It does **not** deduplicate by `id`. The same promotion id can therefore appear multiple times when the operator registered it with multiple rule rows (one SKU-specific, one channel-wildcard).
- `internal/promotion/selector.go::SelectByPath` (lines 26–37) splits the candidate list into two subsets but explicitly does **not** deduplicate. The package comment (lines 13–16) states: *"A single promotion can be present in BOTH paths when the operator registered the same promotion id with two different rule rows … The Apply step is responsible for any de-duplication."*
- `internal/promotion/resolver.go::Apply` (lines 62–77) walks the merged list and applies every promotion it sees. The comment at lines 57–61 explicitly admits the bug: *"if the same id is visible via both the SKU-specific and the channel-wildcard rule rows, the storage layer will return it twice and this loop will apply it twice."*

**Affected paths (must all be fixed — see corrected requirement 11):**

| Path | Trigger | Effect |
|---|---|---|
| **Interactive quote** (`POST /api/v1/pricing/quote`) | SKU matches a SKU-specific rule row AND a wildcard rule row exists for the same id | Discount applied twice; final price too low |
| **Batch quote** (`POST /api/v1/pricing/batch`) | Same as above, per line item | Same defect, multiplied across the batch |
| **Re-quote / replay** (idempotent on `request_hash`) | First call stored the wrong `final_yen`; replay returns the wrong value | Stale wrong price served from `pricing_decisions` |
| **Audit trail** | Two `audit-<uuid>` events emitted for one logical application | Audit log shows two discounts where the operator registered one |

**Not affected (verified):**

- `MemoryStore` has the same shape (key = `id|sku`), so the defect is present in tests too — the `private/condition_test.go` is the scenario's own detector.
- `pricing_decisions.applied_ids` is a `text[]`; duplicates are stored as-is.

---

## 5. Impact assessment

### 5.1 Interactive

- Customer-facing price is wrong (too low) for any quote where the dual-path condition holds. Revenue leakage.
- The `explanation` array shows the same promotion id twice with two different `before -> after` transitions, which is a visible inconsistency to operators reading the trace.

### 5.2 Batch

- `internal/pricing/batch.go` calls the same `Apply` per line item, so the defect scales linearly with batch size. A 10,000-line batch with 5% dual-path exposure produces 500 over-discounted lines.
- **The corrected requirement explicitly states the duplicate-discount correction must cover interactive and batch pricing.** Both paths share the same `Apply` function, so a single fix in `resolver.go` (or `SelectCandidates`) covers both. The fix must be verified against both paths in the test suite.

### 5.3 Audit

- `audit_events` receives two synthetic `audit-<uuid>` rows per logical application. Downstream reconciliation (finance, BI) will see phantom discounts.
- `pricing_decisions.applied_ids` and `audit_ids` arrays contain duplicates; any consumer that assumes uniqueness is wrong.

### 5.4 Downstream

- Order capture (`POST /api/v1/orders`) reads `final_yen` from the decision; the wrong price flows into the order.
- Invoicing and revenue recognition inherit the error.
- Customer trust: a customer who sees the same discount applied twice on a receipt and is later charged the corrected amount will dispute.

---

## 6. Closely related duplicate-processing risks (within approved scope)

These are not the reported defect, but they share the same shape (same id processed twice) and should be considered when designing the fix:

1. **Promotion stacking across overlapping time windows.** If two rule rows for the same id have overlapping `valid_from`/`valid_until`, both are returned by `SelectCandidates` and both apply. Same root cause as the reported defect.
2. **Promotion stacking across overlapping channels.** A promotion registered for both `web` and `store` channels, queried with `channel='web'`, returns only the `web` row — safe. But a future "channel union" feature would re-introduce the same shape.
3. **Batch replay.** `pricing_decisions` is idempotent on `request_hash` via `ON CONFLICT … DO UPDATE SET final_yen = EXCLUDED.final_yen`. If the first call stored the wrong `final_yen`, every replay returns the wrong value. The fix must include a way to invalidate stale decisions, or the fix must be applied before any wrong decision is persisted.
4. **Audit-event uniqueness.** `audit-<uuid>` is generated per visit, not per logical application. Two visits produce two distinct audit ids even for one logical application. Downstream consumers that key on `audit_id` will see two events.
5. **Override path (future).** The override workflow (§8) will introduce a new write path. If overrides are also subject to promotion stacking, the same defect class can re-emerge there. The override workflow must apply the same dedup invariant.

---

## 7. Missing requirements for the override workflow — RESOLVED in Revision 2

The user has now supplied the corrected requirements. The following table maps each open question from Revision 1 to the resolved answer:

| # | Open question (Revision 1) | Resolved answer (Revision 2) |
|---|---|---|
| 1 | Threshold definition | **Per-tenant, configurable, default JPY 5,000,000.** Absolute yen (integer). Stored in `config_versions` (or a new `tenant_settings` table — see §8.2). |
| 2 | Approver identity | **Two distinct approvers required above threshold.** Requester cannot self-approve. Approver role is `manager`. |
| 3 | Approval state machine | `pending → approved` (requires **two** approvals) / `rejected` / `expired` / `cancelled`. |
| 4 | Persistence | New table `override_requests` (additive migration). |
| 5 | Audit | **Every state transition is audited** — `override.requested`, `override.approved`, `override.rejected`, `override.expired`, `override.cancelled`, plus the final pricing action. |
| 6 | Concurrency | Partial unique index on `(request_hash) WHERE status = 'pending'`. Optimistic update on approve/reject. |
| 7 | Expiry | Configurable TTL (default 24h). Background sweeper. |
| 8 | Notification | Out of scope for v1 (polling only). |
| 9 | Frontend surface | Modal for request, new page for approver queue. |
| 10 | Idempotency | Partial unique index on `(request_hash) WHERE status = 'pending'`. |
| 11 | Rollback | Out of scope for v1. |
| 12 | Threshold change history | Out of scope for v1. |

**Additional requirements from Revision 2 that were not in Revision 1:**

- **Feature flag.** Existing tenant behaviour must remain unchanged until the feature is enabled. The override endpoints must return `404` (or `403 FEATURE_DISABLED`) when the feature is off for a tenant. Default: off for all tenants until explicitly enabled.
- **Backward-compatible migration.** The migration must be additive only — no destructive ALTERs, no renames, no NOT NULL additions to existing tables. New tables and new columns are allowed; existing rows must remain valid.
- **Public API unchanged.** The existing public pricing API (`POST /api/v1/pricing/quote`, `POST /api/v1/pricing/batch`, etc.) must not change. Overrides are exposed via new endpoints only.
- **Independent traceability.** The defect correction and the enhancement must be in separate commits, separate migrations, and separate test files. A reviewer must be able to revert one without affecting the other.
- **Protected tests.** `private/condition_test.go`, `private/clean_baseline_test.go`, `tests/public/*`, and `internal/promotion/duplicate_discount_test.go` must not be weakened, deleted, or skipped. The fix must turn them green; the enhancement must not turn them red.
- **No repository promotion.** No `git push`, no PR, no merge. All work stays in the local checkout.

---

## 8. Impact analysis (override workflow, revised)

### 8.1 APIs

| Endpoint | Method | Role | Purpose | New? |
|---|---|---|---|---|
| `/api/v1/pricing/quote` | POST | any | Interactive pricing | **UNCHANGED** |
| `/api/v1/pricing/batch` | POST | any | Batch pricing | **UNCHANGED** |
| `/api/v1/overrides` | POST | `operator` | Submit an override request | NEW |
| `/api/v1/overrides/{id}` | GET | `operator`, `manager` | Read a single request | NEW |
| `/api/v1/overrides?status=pending` | GET | `manager` | List pending requests | NEW |
| `/api/v1/overrides/{id}/approve` | POST | `manager` | Record one approval | NEW |
| `/api/v1/overrides/{id}/reject` | POST | `manager` | Reject | NEW |
| `/api/v1/overrides/{id}/cancel` | POST | `operator` (requester only) | Cancel | NEW |

**New error codes:** `OVER_THRESHOLD` (no approval needed; use the normal quote path), `APPROVAL_REQUIRED` (override needed but not yet approved), `APPROVER_NOT_AUTHORIZED` (caller is not a `manager`, or is the requester), `INSUFFICIENT_APPROVALS` (only one of two required approvals recorded), `FEATURE_DISABLED` (override workflow not enabled for this tenant).

**Public API contract:** The existing `POST /api/v1/pricing/quote` response shape is unchanged. The override workflow is exposed via new endpoints only. The quote endpoint does NOT return an override status; the operator must call `GET /api/v1/overrides/{id}` to check status.

### 8.2 Database

**New table `override_requests`** (additive migration `0002_overrides.sql`):

```sql
CREATE TABLE override_requests (
    id              text PRIMARY KEY,
    tenant_id       text NOT NULL REFERENCES tenants(id),
    request_hash    text NOT NULL REFERENCES pricing_decisions(request_hash),
    requested_by    text NOT NULL REFERENCES users(id),
    requested_at    timestamptz NOT NULL DEFAULT NOW(),
    requested_final_yen bigint NOT NULL,
    base_yen        bigint NOT NULL,
    threshold_yen   bigint NOT NULL,
    status          text NOT NULL CHECK (status IN ('pending','approved','rejected','expired','cancelled')),
    expires_at      timestamptz NOT NULL,
    -- Two-approval fields
    approval_count  integer NOT NULL DEFAULT 0 CHECK (approval_count IN (0,1,2)),
    first_approver  text REFERENCES users(id),
    first_approved_at timestamptz,
    second_approver text REFERENCES users(id),
    second_approved_at timestamptz,
    -- Decision fields
    decided_by      text REFERENCES users(id),
    decided_at      timestamptz,
    decision_comment text NOT NULL,  -- mandatory reason
    cancelled_by    text REFERENCES users(id),
    cancelled_at    timestamptz
);

-- Partial unique index: only one PENDING request per request_hash at a time.
CREATE UNIQUE INDEX override_requests_one_pending
    ON override_requests (request_hash)
    WHERE status = 'pending';

-- Audit trail for each approval event.
CREATE TABLE override_approvals (
    id              text PRIMARY KEY,
    override_id     text NOT NULL REFERENCES override_requests(id),
    approver_id     text NOT NULL REFERENCES users(id),
    approved_at     timestamptz NOT NULL DEFAULT NOW(),
    comment         text NOT NULL,
    UNIQUE (override_id, approver_id)  -- one approval per approver per request
);
```

**New audit-event kinds:** `override.requested`, `override.approved` (per approver), `override.rejected`, `override.expired`, `override.cancelled`, `pricing.override_applied` (when the overridden price is actually used in a quote).

**Per-tenant threshold storage:** Two options:

- **Option A (preferred):** Extend `config_versions` to a key/value store. Add columns `key text, value bigint` and a composite PK `(tenant_id, key)`. Backward compatible: existing rows have `key = 'version'` and `value = <current version>`. New rows for `key = 'override_threshold_yen'` carry the threshold.
- **Option B:** New table `tenant_settings(tenant_id, key, value, updated_at)`. Cleaner separation but requires a second migration.

**Recommendation: Option A** — extends an existing table, fewer migrations, and the `config_versions` table is already tenant-scoped.

**Backward compatibility:** The migration is purely additive. No existing column is altered, no existing row is modified, no existing constraint is tightened. A tenant with no `override_threshold_yen` row falls back to the default JPY 5,000,000.

### 8.3 Frontend

- **Quote detail page:** if the operator edits `final_yen` to a value below `threshold_yen`, a "Request override" button appears. Clicking it opens a modal with the reason field (mandatory).
- **Pending approvals page** (`/approvals`): visible to users with role `manager`. Lists pending requests with approve/reject buttons. Shows the current `approval_count` (e.g. "1 of 2 approvals").
- **Status badge:** every quote detail shows the override status (`none`, `pending`, `approved`, `rejected`, `expired`, `cancelled`).
- **Feature flag UI:** when the override workflow is disabled for a tenant, the "Request override" button is hidden and the `/approvals` page returns a "feature not enabled" message.

### 8.4 Audit

Every state transition emits an `audit_events` row with `kind` and a JSON `payload` containing the actor, the before/after, the threshold, and the reason. The audit trail covers:

- `override.requested` — requester, requested_final_yen, threshold_yen, reason.
- `override.approved` (per approver) — approver, comment, approval_count after.
- `override.rejected` — approver, comment.
- `override.expired` — system, expires_at.
- `override.cancelled` — requester.
- `pricing.override_applied` — when the overridden price is used in a quote (links the override id to the request_hash).

### 8.5 Security

- **Role separation.** New role `manager` (already exists in the role model). Auth middleware must distinguish requester from approver.
- **Self-approval prevention.** The approve endpoint checks `requester_id != approver_id` and returns `403 APPROVER_NOT_AUTHORIZED` if they match.
- **Two-approver enforcement.** The approve endpoint checks `approval_count < 2` and `approver_id != first_approver_id` (the same approver cannot record both approvals).
- **CSRF.** State-changing endpoints require a CSRF token (existing middleware).
- **Rate limiting.** Override requests are rate-limited per user (e.g. 10/minute) to prevent abuse.
- **Reason validation.** The `decision_comment` field is mandatory and must be non-empty (min 1 char, max 500 chars). The `reason` field on the request is also mandatory.

### 8.6 Tests

- **Unit tests** for the threshold check, the state machine, the two-approver enforcement, the self-approval prevention, and the concurrency guard.
- **Integration test** for the full request → first approval → second approval → quote flow.
- **Integration test** for the request → reject → fallback to system price flow.
- **Integration test** for the request → expire → cannot approve flow.
- **E2E test** for the operator UI (request) and the approver UI (approve).
- **Backward-compatibility test:** with the feature flag off, the override endpoints return `404` and the existing quote flow is unchanged.
- **Independent traceability:** the defect fix and the enhancement have separate test files. The defect fix tests live in `internal/promotion/duplicate_discount_test.go` (already exists) and `private/condition_test.go` (protected). The enhancement tests live in a new `internal/application/override_service_test.go` and `tests/public/override_test.go`.

### 8.7 Deployment

- **New env vars:**
  - `OVERRIDE_WORKFLOW_ENABLED` (default `false`) — global feature flag. When `false`, all override endpoints return `404`.
  - `OVERRIDE_REQUEST_TTL` (default `24h`) — how long a `pending` request lives before auto-expiry.
  - `OVERRIDE_APPROVER_ROLE` (default `manager`) — role required to approve/reject.
  - `OVERRIDE_DEFAULT_THRESHOLD_YEN` (default `5000000`) — default threshold for tenants that have not been explicitly configured.
- **Migration order:** `0002_overrides.sql` must run before the new code is deployed. The migration is idempotent (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`).
- **Rollback plan:** the new code path is additive. If the override workflow is rolled back, the override endpoints are removed but the `override_requests` and `override_approvals` tables remain (they are harmless without the endpoints). The existing quote flow is unaffected.
- **No repository promotion.** No `git push`, no PR, no merge. All work stays in the local checkout.

---

## 9. Material implementation alternatives

### 9.1 Defect fix

**Alternative A — Deduplicate in `Apply` (resolver.go).** Track `seen map[string]bool`; skip a promotion id that has already been applied. Minimal blast radius; one file changed; preserves the storage layer's "return everything" contract.

**Alternative B — Deduplicate in `SelectCandidates` (storage layer).** Add `SELECT DISTINCT ON (id) …` to the query. Pushes the invariant down to the storage layer; protects every caller, not just `Apply`. Slightly larger blast radius (memory store must match).

**Alternative C — Deduplicate in `SelectByPath` (selector.go).** When the same id appears in both subsets, keep only the SKU-specific row (higher specificity wins). Changes the semantics: a wildcard-only promotion would still apply, but a dual-path promotion would apply only via the SKU-specific row. This is a **behaviour change**, not just a bug fix — operators who registered a wildcard row expecting it to be the canonical rule would see different behaviour.

**Recommendation: Alternative A.** The invariant "a promotion id applies at most once per quote" belongs in the resolver, where the application decision is made. Alternative B is acceptable but pushes the invariant into a layer that should be agnostic to application semantics. Alternative C is a behaviour change and should not be adopted without product sign-off. Alternative A is the smallest change that turns the protected tests green and is independently traceable from the enhancement.

### 9.2 Override workflow

**Alternative X — Synchronous approval (requester waits for approver in the same request).** Simple, but blocks the requester and ties up a connection. Bad UX.

**Alternative Y — Asynchronous approval with polling.** Requester submits, gets a `request_id`, polls for status. Standard pattern. Frontend polls every N seconds.

**Alternative Z — Asynchronous approval with webhook + push notification.** Requester submits, gets a `request_id`, receives a webhook (or in-app push) when the approver acts. Best UX, but requires a webhook delivery subsystem that does not exist today.

**Recommendation: Alternative Y for v1, with the data model designed so Z can be added later without migration.** The webhook subsystem is out of scope for this change.

**Two-approver enforcement alternatives:**

- **Alternative Y1 — Sequential approvals.** First approver records approval 1; second approver records approval 2. The same approver cannot record both. Enforced by `UNIQUE (override_id, approver_id)` on `override_approvals`.
- **Alternative Y2 — Parallel approvals.** Both approvers can record their approval independently. The `approval_count` field tracks how many have been recorded. When it reaches 2, the status transitions to `approved`. Enforced by the same unique constraint.

**Recommendation: Alternative Y2 (parallel).** It is simpler to implement, does not require ordering, and matches the corrected requirement that "overrides above the threshold require two approvals" without specifying an order.

---

## 10. Recommended approach

**Defect:** Alternative A — deduplicate in `Apply` (resolver.go). One-file change: add a `seen map[string]struct{}` and skip already-seen ids. The regression test in `internal/promotion/duplicate_discount_test.go` and the protected test in `private/condition_test.go` will turn green. The fix covers both interactive and batch pricing because both paths call the same `Apply` function.

**Enhancement:** Alternative Y2 — asynchronous approval with polling, two parallel approvals. New `override_requests` and `override_approvals` tables, new endpoints, new frontend page. Threshold is per-tenant, configurable via `config_versions`, default JPY 5,000,000. Feature flag `OVERRIDE_WORKFLOW_ENABLED` defaults to `false` so existing tenant behaviour is unchanged until the feature is enabled. Approver role is `manager`. Request TTL is 24 hours. Every state transition is audited.

**Independent traceability:** the defect fix and the enhancement are in separate commits, separate migrations (`0002_overrides.sql` is additive only), and separate test files. A reviewer can revert one without affecting the other.

**No repository promotion.** All work stays in the local checkout. No `git push`, no PR, no merge.

---

## 11. Evaluators and acceptance criteria

### 11.1 Defect

| Evaluator | Acceptance criterion |
|---|---|
| `TestDuplicateDiscount_DualPathPromotion` | `promo-0004` appears exactly once in `appliedIDs` for a dual-path quote. **PROTECTED.** |
| `TestDuplicateDiscount_OnlyWildcardMatches` | Wildcard-only path still applies the promotion exactly once. **PROTECTED.** |
| `TestDuplicateDiscount_DistinctIDsUnaffected` | Two distinct ids both apply. **PROTECTED.** |
| `private/condition_test.go::TestCondition_MAINT_DEF_01` | The scenario's own detector turns green. **PROTECTED.** |
| `private/condition_test.go::TestCondition_MAINT_DEF_01_ResolverDirect` | The resolver-direct detector turns green. **PROTECTED.** |
| `private/clean_baseline_test.go::TestCleanBaseline_DuplicateOnce` | The clean-baseline reference still passes. **PROTECTED.** |
| `tests/public/pricing_test.go::TestInteractivePricing_RoundTrip` | Interactive quote still returns 900 yen for the public fixture. **PROTECTED.** |
| `tests/public/pricing_test.go::TestInteractiveAndBatchAgree` | Interactive and batch agree on the final amount. **PROTECTED.** |
| `tests/public/pricing_test.go::TestPromotion_InclusiveEndTime` | Inclusive end-of-window behaviour preserved. **PROTECTED.** |
| `tests/public/pricing_test.go::TestPromotion_NoOverlap` | Single wildcard promotion applies exactly once. **PROTECTED.** |
| `tests/public/batch_test.go::TestBatch_ProducesStableHash` | Batch result hash is stable. **PROTECTED.** |
| `tests/public/batch_test.go::TestBatch_EmptyInput` | Empty batch returns empty result. **PROTECTED.** |
| `tests/public/promotion_test.go::TestPromotion_CRUD` | Promotion CRUD still works. **PROTECTED.** |
| `tests/public/promotion_test.go::TestPromotion_ListIsChannelScoped` | Promotion list is channel-scoped. **PROTECTED.** |
| `tests/public/health_test.go::TestHealthz` | Healthz returns 200. **PROTECTED.** |
| `tests/public/health_test.go::TestVersion` | Version returns the build commit. **PROTECTED.** |
| Live curl against the running API | `POST /api/v1/pricing/quote` for `tenant-a / SKU-00020 / web` returns `applied_promotion_ids` with `promo-0004` exactly once. |
| Live curl against the running API (batch) | `POST /api/v1/pricing/batch` for the same SKU returns the same corrected final amount. |
| Audit-event count | `audit_events` for the same `request_hash` contains exactly one row per logical application. |

### 11.2 Override workflow

| Evaluator | Acceptance criterion |
|---|---|
| Unit: threshold check | A quote with `final_yen < threshold_yen` requires an approved override before the order can be placed. |
| Unit: default threshold | A tenant with no `override_threshold_yen` row uses the default JPY 5,000,000. |
| Unit: per-tenant threshold | Tenant A with threshold 5,000,000 and Tenant B with threshold 1,000,000 are evaluated independently. |
| Unit: state machine | `pending → approved` (requires 2 approvals), `pending → rejected`, `pending → expired`, `pending → cancelled` are the only valid transitions from `pending`; `approved`, `rejected`, `expired`, `cancelled` are terminal. |
| Unit: self-approval | The requester cannot approve their own request (returns `APPROVER_NOT_AUTHORIZED`). |
| Unit: same-approver-twice | The same approver cannot record both approvals (returns `APPROVER_NOT_AUTHORIZED`). |
| Unit: mandatory reason | An approve or reject request without a `comment` returns `400 REASON_REQUIRED`. |
| Unit: feature flag off | With `OVERRIDE_WORKFLOW_ENABLED=false`, all override endpoints return `404`. |
| Integration: full flow | Requester submits → first approver approves → second approver approves → quote is served with the overridden `final_yen`. |
| Integration: rejection | Requester submits → approver rejects → quote falls back to the system-computed price. |
| Integration: expiry | A `pending` request older than `OVERRIDE_REQUEST_TTL` is marked `expired` and cannot be approved. |
| Integration: backward compatibility | With the feature flag off, the existing quote flow is byte-for-byte identical to the pre-change flow. |
| E2E: operator UI | The operator can request an override from the quote detail page; the approver can approve from the pending-approvals page. |
| Audit | Every state transition emits an `audit_events` row with the actor, the before/after, the threshold, and the reason. |
| Migration | `0002_overrides.sql` is additive only; existing rows in existing tables are unchanged. |

---

## 12. Approval points

The following decisions require explicit human approval before implementation begins:

1. **Defect fix location.** Alternative A (resolver) vs. Alternative B (storage). Recommendation: A.
2. **Defect fix semantics.** Is "deduplicate by id" the right invariant, or should the SKU-specific row win over the wildcard row when both match (Alternative C)? Recommendation: deduplicate by id; do not change specificity semantics.
3. **Override threshold storage.** Option A (extend `config_versions`) vs. Option B (new `tenant_settings` table). Recommendation: A.
4. **Override threshold default.** JPY 5,000,000 per the corrected requirement. **Confirmed.**
5. **Two-approver enforcement.** Sequential (Y1) vs. parallel (Y2). Recommendation: Y2.
6. **Override request TTL.** 24 hours is a guess. Recommendation: 24 hours, configurable.
7. **Override persistence.** New table vs. extension to `pricing_decisions`. Recommendation: new table.
8. **Frontend surface.** New page vs. modal. Recommendation: modal for request, new page for the approver queue.
9. **Rollout strategy.** Big-bang vs. feature-flagged. Recommendation: feature-flagged (`OVERRIDE_WORKFLOW_ENABLED`), default off in production until the approver training is complete. **Confirmed by corrected requirement 3.**
10. **Independent traceability.** Defect and enhancement in separate commits, separate migrations, separate test files. **Confirmed by corrected requirement 10.**
11. **No repository promotion.** No `git push`, no PR, no merge. **Confirmed by corrected requirement 13.**

---

## 13. What was NOT done (per the user's instruction)

- No source files were modified.
- No new migrations were applied.
- No new endpoints were added.
- No new frontend pages were added.
- No broad refactoring was performed.
- No `git push`, no PR, no merge.

The only file added in the previous session is `internal/promotion/duplicate_discount_test.go`, which is a **test** that pins the defect. It does not change production behaviour; it documents the bug and will turn green when the fix is applied.

---

## 14. Unresolved governance gate (carried forward from Revision 1)

The previous turn ended with a governance gate blocking promotion: `internal/security/redact.go:26` carries an explicit placeholder marker (`[redacted]`) that the placeholder policy flags. This gate is **not blocking the analysis update** (analysis is read-only), but it **will block any future repository promotion**. The user has explicitly stated "no repository promotion is authorised" in the corrected requirements, so the gate is moot for this turn. It is recorded here for completeness so that any future promotion attempt surfaces the same gate.

The placeholder at `redact.go:26` is the literal string `"[redacted]"` returned by `RedactValue` when the key is sensitive. This is intentional behaviour (the function is supposed to return a redacted placeholder), but the placeholder policy treats the literal string as a placeholder marker. The gate would need to be resolved (either by changing the policy, by renaming the constant, or by adding a comment that exempts this specific use) before any promotion is attempted.

---

## 15. Summary of corrected requirements (for traceability)

| # | Requirement | Status |
|---|---|---|
| 1 | Override threshold is configurable per tenant | RESOLVED — stored in `config_versions` |
| 2 | Default threshold is JPY 5,000,000 | RESOLVED — `OVERRIDE_DEFAULT_THRESHOLD_YEN` env var |
| 3 | Existing tenant behaviour unchanged until feature enabled | RESOLVED — `OVERRIDE_WORKFLOW_ENABLED` feature flag, default off |
| 4 | Overrides above threshold require two approvals | RESOLVED — `approval_count` field, `override_approvals` table |
| 5 | Requester cannot approve their own request | RESOLVED — `requester_id != approver_id` check |
| 6 | Approval reason is mandatory | RESOLVED — `decision_comment NOT NULL` |
| 7 | Every request, approval, rejection and final pricing action is audited | RESOLVED — `audit_events` rows for each transition |
| 8 | Existing public pricing API must not change | RESOLVED — overrides are additive only |
| 9 | Migration must be backward compatible | RESOLVED — `0002_overrides.sql` is additive only |
| 10 | Defect correction and enhancement must remain independently traceable | RESOLVED — separate commits, separate migrations, separate test files |
| 11 | Duplicate-discount correction must cover interactive and batch pricing | RESOLVED — single fix in `Apply` covers both paths |
| 12 | Protected tests must not be weakened | RESOLVED — all protected tests listed in §11.1 |
| 13 | No repository promotion is authorised | RESOLVED — no `git push`, no PR, no merge |

**Explicit confirmation:** The tenant-specific threshold (per-tenant, configurable, default JPY 5,000,000) **supersedes any earlier global-threshold assumption** made in Revision 1. There is no global threshold. There is no environment-wide default. The threshold is a per-tenant value, and the default for any tenant that has not been explicitly configured is JPY 5,000,000.
