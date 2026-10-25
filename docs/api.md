# API

All endpoints are served under `/api/v1/`. The transport is JSON over
HTTP. Errors are returned as plain text bodies with the appropriate
4xx or 5xx status code.

## `GET /healthz`

Liveness check.

```http
GET /healthz HTTP/1.1
```

```json
{ "status": "ok" }
```

## `GET /version`

Build identity.

```http
GET /version HTTP/1.1
```

```json
{
  "commit": "abc123",
  "built_at": "2026-08-10T08:00:00Z",
  "dataset_id": ""
}
```

## `GET /metrics`

In-process counters as a JSON object.

## `POST /api/v1/pricing/quote`

Interactive pricing.

```http
POST /api/v1/pricing/quote
Content-Type: application/json

{
  "tenant_id": "tenant-a",
  "customer_id": "cust-001",
  "sku": "SKU-00001",
  "quantity": 1,
  "channel": "web",
  "at": "2026-06-01T00:00:00Z"
}
```

Response (200 OK):

```json
{
  "request_hash": "…",
  "base_yen": 1000,
  "final_yen": 900,
  "applied_promotion_ids": ["promo-public-1"],
  "audit_ids": ["audit-…"],
  "explanation_lines": ["applied promotion promo-public-1 (percent=10): 1,000 -> 900"]
}
```

## `POST /api/v1/pricing/batch`

Batch pricing. Calls the same money functions as the interactive path.

```http
POST /api/v1/pricing/batch
Content-Type: application/json

{
  "tenant_id": "tenant-a",
  "requests": [
    { "tenant_id": "tenant-a", "customer_id": "cust-001", "sku": "SKU-00001", "quantity": 1, "channel": "web" }
  ]
}
```

Response (200 OK):

```json
{
  "job_id": "job-…",
  "result_hash": "…",
  "result": [
    {
      "request": { "tenant_id": "tenant-a", "sku": "SKU-00001", "quantity": 1, "channel": "web" },
      "decision": { "base_yen": 1000, "final_yen": 900, "applied_promotion_ids": ["…"], "audit_ids": ["…"] }
    }
  ]
}
```

## `GET /api/v1/products`

List products for one tenant.

```http
GET /api/v1/products?tenant_id=tenant-a
```

## `GET /api/v1/promotions`

List active promotions. Channel is optional.

```http
GET /api/v1/promotions?tenant_id=tenant-a&channel=web
```

## `POST /api/v1/promotions`

Create a promotion.

```http
POST /api/v1/promotions
Content-Type: application/json

{
  "tenant_id": "tenant-a",
  "name": "Friday bonus",
  "type": "percent",
  "value": 10,
  "channel": "web",
  "sku": "",
  "priority": 5,
  "valid_from": "2026-01-01T00:00:00Z",
  "valid_until": "2027-01-01T00:00:00Z"
}
```

## `PATCH /api/v1/promotions/{id}`

Update a promotion.

## `GET /api/v1/audit`

List recent audit events for a tenant.

```http
GET /api/v1/audit?tenant_id=tenant-a&limit=50
```

## `GET /api/v1/batch_jobs/{id}`

Inspect a batch job.

## `POST /api/v1/admin/seed`

Re-run the deterministic seed.
