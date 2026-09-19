# Pricing defect and override implementation report

## Independent changes

- Defect correction: commit `4b017b4`, resolver-level idempotent de-duplication by promotion identity. The SKU-specific row wins over a wildcard row for the same identity.
- Override enhancement: the commit immediately following `4b017b4`, implementing an opt-in per-tenant lifecycle (`requested`, `awaiting_second_approval`, `approved`, `rejected`, `applied`).
- No repository promotion was performed.

## Defect verification

The reproduction test was copied onto the pre-fix parent and run before production changes. It failed with two applications of `promo-overlap`. On the corrected commit, the same test passed with a JPY 900 final price and one audit identifier. Distinct promotions still stack.

## Enhancement coverage

- Disabled-by-default tenant configuration and JPY 5,000,000 default threshold.
- Stored-user role checks for pricing requesters and admin reviewers.
- Mandatory request, approval, and rejection reasons.
- Requester self-approval prohibition and two distinct reviewers.
- Audited lifecycle transitions and configuration-version invalidation.
- Apply only from approved state and only inside the validity window.
- Memory and PostgreSQL persistence, forward migration, reverse migration, and active-override lookup.
- Interactive and batch pricing integration without changing the existing public pricing API shape.
- Versioned backend endpoints; no frontend changes.

## File-scope justification

- `internal/domain/override.go`: lifecycle and tenant configuration data.
- `internal/override/*`: workflow service and focused tests.
- `internal/storage/{memory,postgres}.go`: persistence and stored-user lookup.
- `migrations/0002_price_overrides*.sql`: forward and rollback schema.
- `internal/pricing/{engine,batch}.go`: apply only active, applied overrides.
- `internal/pricing/override_integration_test.go`: quote/batch behavior.
- `internal/httpapi/{server,override_test}.go`, `cmd/api/main.go`: versioned endpoints and wiring.
- `cmd/migrate/main.go`: execute all numbered migrations in correct direction.
- `docs/api.md`: backend contract.

No frontend files were changed. Baseline files reported by `gofmt -l` but unrelated to this work were intentionally left unchanged.

## Remaining risks

- Workflow writes, audit appends, and config-version bumps are separate repository calls rather than one database transaction. A storage failure between calls can leave state changed without its matching audit record.
- The existing application has no authentication middleware. Endpoint actor IDs are therefore trusted inputs, although roles and tenant membership are checked against stored users. Deployment must place these endpoints behind authenticated identity propagation before production use.
- Concurrent approvals are not protected by optimistic locking or row locks; simultaneous reviewers could race. Database constraints prevent duplicate identities in persisted rows, but serializable transition handling remains future hardening.
- When several active applied overrides match the same tenant/SKU/channel, the most recently applied wins. The policy is deterministic but should be confirmed as the desired business conflict rule.

## Rollback

1. Stop new override requests and applications at the API boundary.
2. Deploy the preceding application revision (`4b017b4`) so no running process depends on override tables.
3. Run `go run ./cmd/migrate -direction down`; migrations execute in reverse order, dropping `price_overrides` and `override_thresholds` before the base schema only when a full rollback is intended.
4. For enhancement-only rollback while preserving the base schema, execute `migrations/0002_price_overrides.down.sql` directly through the controlled migration process.
5. The defect correction can be independently reverted with `git revert 4b017b4` if required; doing so reintroduces the confirmed 810 JPY defect and is not recommended.
