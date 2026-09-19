# Data dictionary

The PostgreSQL schema lives in `migrations/0001_init.sql`. All tables
are tenant-scoped unless otherwise noted.

## `tenants`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | TEXT, PK | Synthetic identifier, e.g. `tenant-a` |
| `name` | TEXT NOT NULL | Human-readable name |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |

## `users`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | TEXT, PK | |
| `tenant_id` | TEXT, FK → `tenants(id)` | |
| `name` | TEXT NOT NULL | |
| `role` | TEXT NOT NULL | `admin`, `pricing` |

## `products`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | TEXT, PK | |
| `tenant_id` | TEXT, FK → `tenants(id)` | |
| `sku` | TEXT NOT NULL | |
| `name` | TEXT NOT NULL | |
| `base_yen` | BIGINT NOT NULL | |
| UNIQUE | `(tenant_id, sku)` | |

## `customers`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | TEXT, PK | |
| `tenant_id` | TEXT, FK | |
| `name` | TEXT NOT NULL | |
| `segment` | TEXT NOT NULL | `retail`, `wholesale`, `vip` |

## `promotions`

A promotion is a logical discount object. The same promotion id can
appear in multiple rows, each scoped to a different SKU (concrete
value for SKU-specific rules, empty string for the channel-wildcard
rule).

| Column | Type | Notes |
| --- | --- | --- |
| `id` | TEXT NOT NULL | Logical promotion id |
| `tenant_id` | TEXT, FK | |
| `name` | TEXT NOT NULL | |
| `type` | TEXT NOT NULL | `percent` or `amount` |
| `value` | DOUBLE PRECISION NOT NULL | percent (0..100) or JPY amount |
| `channel` | TEXT NOT NULL | `web`, `store`, `mobile` |
| `sku` | TEXT NOT NULL DEFAULT '' | empty for wildcard rules |
| `priority` | INTEGER NOT NULL DEFAULT 0 | |
| `valid_from` | TIMESTAMPTZ NOT NULL | INCLUSIVE |
| `valid_until` | TIMESTAMPTZ NOT NULL | INCLUSIVE |
| PK | `(id, sku)` | |

## `pricing_decisions`

| Column | Type | Notes |
| --- | --- | --- |
| `request_hash` | TEXT, PK | |
| `base_yen` | BIGINT NOT NULL | |
| `final_yen` | BIGINT NOT NULL | |
| `applied_ids` | TEXT[] NOT NULL DEFAULT '{}' | |
| `audit_ids` | TEXT[] NOT NULL DEFAULT '{}' | |
| `explanation` | TEXT[] NOT NULL DEFAULT '{}' | |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT NOW() | |

## `batch_jobs`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | TEXT, PK | |
| `tenant_id` | TEXT, FK | |
| `created_at` | TIMESTAMPTZ NOT NULL | |
| `total_rows` | INTEGER NOT NULL DEFAULT 0 | |
| `done_rows` | INTEGER NOT NULL DEFAULT 0 | |
| `status` | TEXT NOT NULL | `pending`, `running`, `done` |
| `result_hash` | TEXT | |

## `audit_events`

| Column | Type | Notes |
| --- | --- | --- |
| `id` | TEXT, PK | |
| `tenant_id` | TEXT, FK | |
| `action` | TEXT NOT NULL | `promotion.created`, `promotion.applied`, … |
| `subject` | TEXT NOT NULL | typically a promotion id or SKU |
| `detail` | TEXT NOT NULL | human-readable summary |
| `created_at` | TIMESTAMPTZ NOT NULL | |

## `config_versions`

| Column | Type | Notes |
| --- | --- | --- |
| `tenant_id` | TEXT, PK, FK | |
| `version` | BIGINT NOT NULL DEFAULT 1 | bumped on any change that may affect pricing |
