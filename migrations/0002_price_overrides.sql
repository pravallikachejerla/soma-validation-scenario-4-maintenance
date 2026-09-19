-- Forward migration: Scenario 4 manual price-override workflow.
-- Adds two tables: per-tenant override thresholds (gate config) and the
-- override request lifecycle rows.
--
-- Schema is aligned with internal/storage/postgres.go's OverrideRepo +
-- OverrideThresholdRepo implementations and internal/domain/override.go:
--   - one reason column (override.reason) set at request time
--   - one reject_reason column set when an approver rejects
--   - approver role-separation is enforced in the service layer (requester !=
--     approver1 != approver2), not in CHECK constraints, so the columns can
--     be nullable during the requested / awaiting_second_approval states.
--
-- Idempotent (CREATE IF NOT EXISTS) so the seeder can re-run safely.
-- See migrations/0002_price_overrides.down.sql for the rollback.

CREATE TABLE IF NOT EXISTS override_thresholds (
    tenant_id     TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    enabled       BOOLEAN     NOT NULL DEFAULT FALSE,
    threshold_yen BIGINT      NOT NULL DEFAULT 5000000 CHECK (threshold_yen > 0),
    updated_at    TIMESTAMPTZ NOT NULL,
    updated_by    TEXT        NOT NULL REFERENCES users(id)
);

CREATE TABLE IF NOT EXISTS overrides (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    sku           TEXT        NOT NULL,
    channel       TEXT        NOT NULL,
    quantity      BIGINT      NOT NULL CHECK (quantity > 0),
    override_yen  BIGINT      NOT NULL CHECK (override_yen > 0),
    requester_id  TEXT        NOT NULL REFERENCES users(id),
    approver1_id  TEXT                 REFERENCES users(id),
    approver2_id  TEXT                 REFERENCES users(id),
    state         TEXT        NOT NULL CHECK (state IN ('requested','awaiting_second_approval','approved','rejected','applied')),
    reason        TEXT        NOT NULL,
    reject_reason TEXT,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    valid_from    TIMESTAMPTZ NOT NULL,
    valid_until   TIMESTAMPTZ NOT NULL,
    applied_at    TIMESTAMPTZ,
    CHECK (valid_until > valid_from)
);

CREATE INDEX IF NOT EXISTS overrides_active_idx
    ON overrides (tenant_id, sku, channel, state, valid_from, valid_until);
