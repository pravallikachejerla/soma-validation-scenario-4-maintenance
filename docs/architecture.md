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
