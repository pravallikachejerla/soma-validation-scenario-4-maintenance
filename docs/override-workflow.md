# Override Workflow — Specification (REVISION 2, pending approval)

**Status:** Draft. No implementation. Awaiting approval on the points listed in `ANALYSIS.md` §12.

## 1. Purpose

Allow an operator to manually override the system-computed `final_yen` for a pricing quote, **only when** the override is above a configured per-tenant threshold and **only when** two designated approvers have approved the request.

## 2. Threshold

- **Unit:** absolute yen (integer).
- **Scope:** **per-tenant.** There is no global threshold. The tenant-specific threshold supersedes any earlier global-threshold assumption.
- **Default:** **JPY 5,000,000** for any tenant that has not been explicitly configured.
- **Storage:** `config_versions` table, extended to a key/value store. Key `override_threshold_yen`, value `bigint`.
- **Read path:** loaded once at API startup, cached, refreshed on a `config_versions` insert (out of scope for v1 — restart required).
- **Configurable at runtime:** yes, via `POST /api/v1/admin/tenants/{id}/settings` (out of scope for v1 — manual SQL for now).

## 3. Feature flag

- **Env var:** `OVERRIDE_WORKFLOW_ENABLED` (default `false`).
- **Behaviour when off:** all override endpoints return `404`. The existing quote flow is byte-for-byte identical to the pre-change flow.
- **Behaviour when on:** the override endpoints are active. Existing tenant behaviour is unchanged until the tenant is explicitly configured (i.e. a tenant with no `override_threshold_yen` row uses the default JPY 5,000,000, but the override endpoints are still gated by the global feature flag).
- **Per-tenant enablement (future):** out of scope for v1. The global flag is sufficient.

## 4. State machine

```
            submit
              │
              ▼
          ┌────────┐  approve(1)  ┌──────────────┐
          │pending │ ───────────► │ pending      │
          │        │              │ (1 approval) │
          │        │  approve(2)  └──────────────┘
          │        │ ───────────► │ approved     │ (terminal)
          │        │              └──────────────┘
          │        │  reject      ┌─────────┐
          │        │ ───────────► │rejected │ (terminal)
          │        │              └─────────┘
          │        │  expire      ┌─────────┐
          │        │ ───────────► │ expired │ (terminal)
          │        │              └─────────┘
          │        │  cancel      ┌─────────┐
          └────────┘ ───────────► │cancelled│ (terminal)
                                   └─────────┘
```

- `submit` — requester (any authenticated user with role `operator`).
- `approve(1)` / `approve(2)` — approver (role `manager`). **Two distinct approvers required.** Requester cannot self-approve. Same approver cannot record both approvals.
- `reject` — approver (role `manager`). Terminal.
- `expire` — automatic, when `now() > expires_at` and status is `pending`. Terminal.
- `cancel` — requester, only while status is `pending`. Terminal.

## 5. Endpoints

| Method | Path | Role | Purpose |
|---|---|---|---|
| `POST` | `/api/v1/overrides` | `operator` | Submit an override request. |
| `GET` | `/api/v1/overrides/{id}` | `operator`, `manager` | Read a single request. |
| `GET` | `/api/v1/overrides?status=pending` | `manager` | List pending requests. |
| `POST` | `/api/v1/overrides/{id}/approve` | `manager` | Record one approval. |
| `POST` | `/api/v1/overrides/{id}/reject` | `manager` | Reject. |
| `POST` | `/api/v1/overrides/{id}/cancel` | `operator` (requester only) | Cancel. |

### 5.1 `POST /api/v1/overrides`

Request:
```json
{
  "tenant_id": "tenant-a",
  "request_hash": "24c1f4e5...",
  "requested_final_yen": 2500,
  "reason": "customer loyalty adjustment"
}
```

Response (201):
```json
{
  "id": "ovr-...",
  "status": "pending",
  "requested_by": "user-123",
  "requested_at": "2026-06-15T12:00:00Z",
  "expires_at": "2026-06-16T12:00:00Z",
  "threshold_yen": 5000000,
  "approval_count": 0
}
```

Errors:
- `400 REASON_REQUIRED` — `reason` is empty or missing.
- `400 OVER_THRESHOLD` — `requested_final_yen >= threshold_yen` (no approval needed; use the normal quote path).
- `404 FEATURE_DISABLED` — `OVERRIDE_WORKFLOW_ENABLED` is `false`.
- `404 QUOTE_NOT_FOUND` — `request_hash` does not exist.
- `409 DUPLICATE_REQUEST` — an active (pending) override already exists for this `request_hash`.

### 5.2 `POST /api/v1/overrides/{id}/approve`

Request:
```json
{ "comment": "approved per policy 7.3" }
```

Response (200):
```json
{
  "id": "ovr-...",
  "status": "pending",
  "approval_count": 1,
  "first_approver": "user-456",
  "first_approved_at": "2026-06-15T13:00:00Z"
}
```

Or, if this is the second approval:
```json
{
  "id": "ovr-...",
  "status": "approved",
  "approval_count": 2,
  "first_approver": "user-456",
  "first_approved_at": "2026-06-15T13:00:00Z",
  "second_approver": "user-789",
  "second_approved_at": "2026-06-15T13:05:00Z",
  "decided_by": "user-789",
  "decided_at": "2026-06-15T13:05:00Z"
}
```

Errors:
- `400 REASON_REQUIRED` — `comment` is empty or missing.
- `403 APPROVER_NOT_AUTHORIZED` — caller is not a `manager`, or is the requester, or has already recorded an approval for this request.
- `404 FEATURE_DISABLED` — `OVERRIDE_WORKFLOW_ENABLED` is `false`.
- `409 INVALID_STATE` — current status is not `pending`.

### 5.3 `POST /api/v1/overrides/{id}/reject`

Request:
```json
{ "comment": "exceeds policy limit" }
```

Response (200):
```json
{
  "id": "ovr-...",
  "status": "rejected",
  "decided_by": "user-456",
  "decided_at": "2026-06-15T13:00:00Z",
  "decision_comment": "exceeds policy limit"
}
```

Errors:
- `400 REASON_REQUIRED` — `comment` is empty or missing.
- `403 APPROVER_NOT_AUTHORIZED` — caller is not a `manager`, or is the requester.
- `404 FEATURE_DISABLED` — `OVERRIDE_WORKFLOW_ENABLED` is `false`.
- `409 INVALID_STATE` — current status is not `pending`.

## 6. Database

New table `override_requests` (additive migration `0002_overrides.sql`):

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
    decision_comment text NOT NULL,
    cancelled_by    text REFERENCES users(id),
    cancelled_at    timestamptz
);

-- Partial unique index: only one PENDING request per request_hash at a time.
CREATE UNIQUE INDEX override_requests_one_pending
    ON override_requests (request_hash)
    WHERE status = 'pending';
```

New table `override_approvals` (one row per approval event):

```sql
CREATE TABLE override_approvals (
    id              text PRIMARY KEY,
    override_id     text NOT NULL REFERENCES override_requests(id),
    approver_id     text NOT NULL REFERENCES users(id),
    approved_at     timestamptz NOT NULL DEFAULT NOW(),
    comment         text NOT NULL,
    UNIQUE (override_id, approver_id)  -- one approval per approver per request
);
```

Extend `config_versions` to a key/value store (additive migration `0002_overrides.sql`):

```sql
-- Backward compatible: existing rows have key='version' and value=<current version>.
-- New rows for key='override_threshold_yen' carry the per-tenant threshold.
ALTER TABLE config_versions ADD COLUMN key text NOT NULL DEFAULT 'version';
ALTER TABLE config_versions ADD COLUMN value bigint;
-- The original version column is preserved for backward compatibility.
ALTER TABLE config_versions ADD CONSTRAINT config_versions_pk_new PRIMARY KEY (tenant_id, key);
-- Drop the old PK constraint if it exists (idempotent).
ALTER TABLE config_versions DROP CONSTRAINT IF EXISTS config_versions_pkey;
```

**Backward compatibility note:** the migration is purely additive. No existing column is altered in a destructive way, no existing row is modified, no existing constraint is tightened. A tenant with no `override_threshold_yen` row falls back to the default JPY 5,000,000.

New audit-event kinds: `override.requested`, `override.approved` (per approver), `override.rejected`, `override.expired`, `override.cancelled`, `pricing.override_applied`.

## 7. Concurrency

- The partial unique index `override_requests_one_pending` prevents two simultaneous `pending` requests for the same `request_hash` at the database level.
- Approve/reject use `UPDATE … WHERE status = 'pending'` and check the row count; if zero rows updated, return `409 INVALID_STATE`.
- The `UNIQUE (override_id, approver_id)` constraint on `override_approvals` prevents the same approver from recording two approvals for the same request.

## 8. Expiry

- A background goroutine in the API process scans for `status = 'pending' AND expires_at < NOW()` every 60 seconds and marks them `expired`.
- Out of scope for v1: distributed expiry (multiple API replicas). Acceptable for single-replica deployments.

## 9. Frontend

- **Quote detail page:** if the operator edits `final_yen` to a value below `threshold_yen`, a "Request override" button appears. Clicking it opens a modal with the reason field (mandatory).
- **Pending approvals page** (`/approvals`): visible to users with role `manager`. Lists pending requests with approve/reject buttons. Shows the current `approval_count` (e.g. "1 of 2 approvals").
- **Status badge:** every quote detail shows the override status (`none`, `pending`, `approved`, `rejected`, `expired`, `cancelled`).
- **Feature flag UI:** when `OVERRIDE_WORKFLOW_ENABLED` is `false`, the "Request override" button is hidden and the `/approvals` page returns a "feature not enabled" message.

## 10. Configuration

| Env var | Default | Purpose |
|---|---|---|
| `OVERRIDE_WORKFLOW_ENABLED` | `false` | Global feature flag. When `false`, all override endpoints return `404`. |
| `OVERRIDE_REQUEST_TTL` | `24h` | How long a `pending` request lives before auto-expiry. |
| `OVERRIDE_APPROVER_ROLE` | `manager` | Role required to approve/reject. |
| `OVERRIDE_DEFAULT_THRESHOLD_YEN` | `5000000` | Default threshold for tenants that have not been explicitly configured. |

## 11. Audit

Every state transition emits an `audit_events` row with `kind` and a JSON `payload`:

| Event | Payload fields |
|---|---|
| `override.requested` | `requester_id`, `requested_final_yen`, `threshold_yen`, `reason` |
| `override.approved` (per approver) | `approver_id`, `comment`, `approval_count` after |
| `override.rejected` | `approver_id`, `comment` |
| `override.expired` | `expires_at` |
| `override.cancelled` | `requester_id` |
| `pricing.override_applied` | `override_id`, `request_hash`, `final_yen` |

## 12. Out of scope for v1

- Webhook delivery on approval/rejection.
- Multi-step approval chains (more than two approvers).
- Cross-tenant overrides.
- Distributed expiry (multiple API replicas).
- Hot-reload of `config_versions`.
- Per-tenant feature flag (only the global flag is supported in v1).
- Rollback of an approved override after the order is placed.
- Threshold change history (existing pending requests do not re-evaluate when the threshold changes).

## 13. Independent traceability

The defect correction and the override workflow enhancement are **independently traceable**:

- **Separate commits.** The defect fix is one commit; the override workflow is a separate commit (or set of commits).
- **Separate migrations.** The defect fix does not require a migration. The override workflow uses `0002_overrides.sql` (additive only).
- **Separate test files.** The defect fix tests live in `internal/promotion/duplicate_discount_test.go` (already exists) and `private/condition_test.go` (protected). The override workflow tests live in a new `internal/application/override_service_test.go` and `tests/public/override_test.go`.
- **A reviewer can revert one without affecting the other.** Reverting the override workflow commit leaves the defect fix in place and the protected tests green. Reverting the defect fix commit leaves the override workflow functional (the override workflow does not depend on the dedup fix).

## 14. No repository promotion

No `git push`, no PR, no merge. All work stays in the local checkout. The override workflow is delivered as a set of local commits that can be reviewed and tested locally, but not promoted to the remote repository.
