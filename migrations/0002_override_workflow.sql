-- Migration 0002: override workflow (additive, backward-compatible)
--
-- Creates four new tables for the manual price-override approval workflow.
-- Existing tables are untouched. The feature is disabled by default per
-- tenant (feature_enabled defaults to false), so existing tenants see
-- no behaviour change until the feature is explicitly enabled.

CREATE TABLE IF NOT EXISTS override_settings (
    tenant_id          TEXT PRIMARY KEY,
    threshold_yen      BIGINT NOT NULL DEFAULT 5000000,
    required_approvals INT NOT NULL DEFAULT 2,
    feature_enabled    BOOLEAN NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS override_requests (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    requester_id  TEXT NOT NULL,
    sku           TEXT NOT NULL,
    channel       TEXT NOT NULL,
    base_price_yen BIGINT NOT NULL,
    override_yen  BIGINT NOT NULL,
    reason        TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending',
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS override_approvals (
    id            SERIAL PRIMARY KEY,
    request_id    TEXT NOT NULL REFERENCES override_requests(id),
    approver_id   TEXT NOT NULL,
    decision      TEXT NOT NULL,
    reason        TEXT NOT NULL,
    at            TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(request_id, approver_id)
);

CREATE TABLE IF NOT EXISTS override_audit (
    id            SERIAL PRIMARY KEY,
    request_id    TEXT NOT NULL,
    action        TEXT NOT NULL,
    actor_id      TEXT NOT NULL,
    reason        TEXT NOT NULL,
    at            TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_override_requests_tenant ON override_requests(tenant_id);
CREATE INDEX IF NOT EXISTS idx_override_audit_request ON override_audit(request_id);
