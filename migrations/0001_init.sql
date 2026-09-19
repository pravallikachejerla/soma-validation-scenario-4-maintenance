-- Forward migration: Scenario 4 stable pricing schema.
-- Idempotent (CREATE IF NOT EXISTS) so the seeder can re-run safely.

CREATE TABLE IF NOT EXISTS tenants (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    role        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS products (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    sku         TEXT NOT NULL,
    name        TEXT NOT NULL,
    base_yen    BIGINT NOT NULL,
    UNIQUE (tenant_id, sku)
);

CREATE TABLE IF NOT EXISTS customers (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    segment     TEXT NOT NULL
);

-- A promotion is a logical discount object. The same promotion id can
-- appear in multiple rule rows, each scoped to a different SKU
-- (concrete value for SKU-specific rules, empty string for the
-- channel-wildcard rule). This is what allows one promotion to qualify
-- through both a SKU-specific path and a wildcard channel path.
CREATE TABLE IF NOT EXISTS promotions (
    id           TEXT NOT NULL,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    type         TEXT NOT NULL CHECK (type IN ('percent', 'amount')),
    value        DOUBLE PRECISION NOT NULL,
    channel      TEXT NOT NULL,
    sku          TEXT NOT NULL DEFAULT '',
    priority     INTEGER NOT NULL DEFAULT 0,
    valid_from   TIMESTAMPTZ NOT NULL,
    valid_until  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (id, sku)
);

CREATE INDEX IF NOT EXISTS promotions_tenant_channel_idx
    ON promotions (tenant_id, channel, valid_from, valid_until);

CREATE TABLE IF NOT EXISTS pricing_decisions (
    request_hash  TEXT PRIMARY KEY,
    base_yen      BIGINT NOT NULL,
    final_yen     BIGINT NOT NULL,
    applied_ids   TEXT[] NOT NULL DEFAULT '{}',
    audit_ids     TEXT[] NOT NULL DEFAULT '{}',
    explanation   TEXT[] NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS batch_jobs (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    total_rows   INTEGER NOT NULL DEFAULT 0,
    done_rows    INTEGER NOT NULL DEFAULT 0,
    status       TEXT NOT NULL DEFAULT 'pending',
    result_hash  TEXT
);

CREATE TABLE IF NOT EXISTS audit_events (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    action      TEXT NOT NULL,
    subject     TEXT NOT NULL,
    detail      TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS audit_events_tenant_idx
    ON audit_events (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS config_versions (
    tenant_id   TEXT PRIMARY KEY REFERENCES tenants(id) ON DELETE CASCADE,
    version     BIGINT NOT NULL DEFAULT 1
);
