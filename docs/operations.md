# Operations

## Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `DATABASE_URL` | `postgres://pricing:pricing@localhost:5432/pricing?sslmode=disable` | Postgres DSN |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `BUILD_COMMIT` | `dev` | Commit hash returned by `/version` |
| `STORAGE_BACKEND` | `""` | If set to `memory`, skip Postgres and use the in-process store |

## Health and readiness

- `/healthz` returns 200 once the HTTP server is up. The endpoint
  does not require the database to be reachable.

## Logging

Every log line is JSON with the following fields:

- `ts` (RFC3339Nano UTC)
- `level` (`info`, `warn`, `error`)
- `service` (`api`, `seed`, `migrate`, …)
- `msg` (short event name)
- `request_id` (synthetic UUID per request)
- `tenant_id` (extracted from the query string or request body)
- any caller-supplied fields, redacted by `security.RedactValue`

Sensitive keys (`customer_id`, `negotiated_price`, `discount_reason`,
`password`, `secret`, `token`, `raw_request`, `raw_response`) are
replaced by `[redacted]` before the line is emitted.

## Metrics

The `/metrics` endpoint exposes the following counters as a JSON
object:

- `pricing_requests_total`
- `pricing_cache_hits_total`
- `pricing_candidate_count`
- `batch_jobs_total`
- `pricing_query_count_total`

## Backups and migrations

`migrations/0001_init.sql` is the forward migration. Its reverse is
`migrations/0001_init.down.sql`. Run them with `make migrate` and
`make migrate-down` respectively. Both are idempotent at the
statement level (using `CREATE TABLE IF NOT EXISTS`).

## Docker Compose

`docker compose up --build` brings the stack up. Services:

- `db` — Postgres 16 with a `pgdata` volume and a healthcheck.
- `migrate` — runs `0001_init.sql` once, then exits.
- `api` — the Go API, exposed on port `8080`, depends on `migrate`.
- `seed` — runs the deterministic seeder.
- `frontend` — Vite preview, exposed on port `5173`.

## Synthetic data

The default profile installs 3 tenants × (200 products + 50 customers
+ 300 promotions). The same dataset can be re-installed at any time
with `make seed` or `POST /api/v1/admin/seed`; the generator uses a
fixed PRNG seed (`42`).
