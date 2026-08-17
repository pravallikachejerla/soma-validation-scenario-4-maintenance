# SOMA Pricing — Scenario 4

A stable, full-stack pricing application. The repository is the
**model-visible input** for SOMA Genesis Scenario 4: Targeted Defect
Resolution, Customisation and Feature Enhancement. It contains a
working pricing stack (Go + React + PostgreSQL) and a single,
reproducible defect that the agent is expected to identify and
correct as part of the scenario.

## Stack

- **Backend**: Go 1.22+ services, versioned REST API, structured JSON
  logging, parameterised SQL, integer JPY money functions, an
  in-process pricing-result cache.
- **Database**: PostgreSQL 16 with versioned, reversible migrations.
- **Frontend**: React 18 + TypeScript + Vite. Role-aware navigation,
  five pages: Products, Promotion editor, Pricing simulator, Batch
  dashboard, Audit viewer.
- **Deployment**: Docker Compose, `Makefile`, multi-stage `Dockerfile`.

## Quick start

```bash
make build      # compile all Go packages
make test       # run the public test suite
make up         # start the full stack with docker-compose
make seed       # populate the synthetic dataset (idempotent)
make migrate    # apply database migrations
```

Then open <http://localhost:5173> for the frontend and
<http://localhost:8080/healthz> for the API health check.

## Public API

| Method | Path | Purpose |
| --- | --- | --- |
| GET    | `/healthz` | Service liveness |
| GET    | `/version` | Build identity (commit, build time) |
| GET    | `/metrics` | In-process counters |
| POST   | `/api/v1/pricing/quote` | Interactive pricing for one request |
| POST   | `/api/v1/pricing/batch` | Batch pricing (calls the same money functions) |
| GET    | `/api/v1/products?tenant_id=…` | List products for a tenant |
| GET    | `/api/v1/promotions?tenant_id=…&channel=…` | List promotions |
| POST   | `/api/v1/promotions` | Create a promotion |
| PATCH  | `/api/v1/promotions/{id}` | Update a promotion |
| GET    | `/api/v1/audit?tenant_id=…&limit=…` | Recent audit events |
| GET    | `/api/v1/batch_jobs/{id}` | Inspect a batch job |
| POST   | `/api/v1/admin/seed` | Re-seed the synthetic dataset |

The full request and response shapes live in `docs/api.md`.

## Role model

The Scenario 4 surface supports a minimal role model — `admin`,
`pricing`. Authentication is delegated to the operator environment; the
repository ships no credential store, no SSO integration and no
session management.

## Dataset profiles

The seed command installs the **default profile**:

- 3 tenants (`tenant-a`, `tenant-b`, `tenant-c`)
- 200 products per tenant
- 50 customers per tenant
- 300 promotions per tenant across `web`, `store`, `mobile` channels

The same seed is reproducible (fixed PRNG seed `42`); running `make
seed` twice produces the same dataset.

A second profile (`SEED_PROFILE=small`) produces a one-tenant, five-SKU
dataset for quick smoke tests.

## Out of scope

The Scenario 4 input does **not** ship a manual price-override
workflow. Manual approval workflows are deliberately not modelled in
this baseline and are not a current user-visible feature.

## Repository layout

```
scenario-4-pricing-defect/
├── cmd/                  # api, seed, migrate entry points
├── internal/             # domain, application, storage, pricing, promotion, money, observability, security
├── migrations/           # 0001_init.sql + 0001_init.down.sql
├── frontend/             # React + Vite + TS frontend
├── tests/public/         # public Go tests
├── docs/                 # architecture, api, operations, data-dictionary, testing
├── deploy/               # Dockerfiles
├── docker-compose.yml
├── Makefile
├── go.mod / go.sum
├── package.json / package-lock.json
├── SBOM.cdx.json
└── README.md
```

## Testing

```bash
make test-public    # public Go tests
make test-private   # private evaluator tests
```

The public test suite is a normal engineering check; the private suite
is reserved for the scenario evaluator and is not part of the
model-visible surface.

## License

Internal synthetic validation asset — not for production use.
