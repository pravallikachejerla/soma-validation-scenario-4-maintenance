# Verification Report — Scenario 4 Pricing repo

**Repo under test:** `/workspace/scenario-4-pricing-defect/`
**Verdict:** PASS

All required checks were re-executed independently by the verifier (not by
re-reading the producer's deliverable). The defect is genuinely planted and
detectable; the override feature is genuinely absent.

---

## 1. Build sanity

### Check: `go build ./...`
**Method:** `cd /workspace/scenario-4-pricing-defect && go build ./...`
**Evidence:**
```
$ go build ./...
$ echo $?
0
```
**Result: PASS**

The repo builds clean. Go toolchain auto-fetched 1.25.0 because
`go.mod` declares `go 1.25.0`; the sandbox's local 1.22.5 was not used
directly but the toolchain mechanism worked.

---

## 2. Public test sanity

### Check: `go test ./tests/public/...`
**Method:** `cd /workspace/scenario-4-pricing-defect && go test -v -count=1 ./tests/public/...`
**Evidence:**
```
=== RUN   TestBatch_ProducesStableHash              --- PASS
=== RUN   TestBatch_EmptyInput                      --- PASS
=== RUN   TestHealthz                               --- PASS
=== RUN   TestVersion                               --- PASS
=== RUN   TestInteractivePricing_RoundTrip          --- PASS
=== RUN   TestInteractiveAndBatchAgree              --- PASS
=== RUN   TestPromotion_InclusiveEndTime            --- PASS
=== RUN   TestPromotion_NoOverlap                   --- PASS
=== RUN   TestPromotion_CRUD                        --- PASS
=== RUN   TestPromotion_ListIsChannelScoped         --- PASS
PASS
ok  	github.com/somagen/scenario4/tests/public	0.004s
```
**Result: PASS** — all 10 public tests pass (the producer's table says "4
tests" but the suite actually has 10, all of which pass; the discrepancy is
cosmetic).

### Check: no MAINT/DEF/defect/duplicate/double/de-dup in public test names
**Method:**
```
grep -E "^func Test" tests/public/*.go
```
**Evidence:**
```
batch_test.go:     TestBatch_ProducesStableHash
batch_test.go:     TestBatch_EmptyInput
health_test.go:    TestHealthz
health_test.go:    TestVersion
pricing_test.go:   TestInteractivePricing_RoundTrip
pricing_test.go:   TestInteractiveAndBatchAgree
pricing_test.go:   TestPromotion_InclusiveEndTime
pricing_test.go:   TestPromotion_NoOverlap
promotion_test.go: TestPromotion_CRUD
promotion_test.go: TestPromotion_ListIsChannelScoped
```
**Result: PASS** — no public test name references the planted defect or
the absent feature. The closest is `TestPromotion_NoOverlap`, which uses
the term only to assert that a single non-overlapping rule is applied
exactly once.

---

## 3. Private test runs

### Check: `go test ./private/...` — `TestCondition_MAINT_DEF_01` compiles and fails on seeded source
**Method:** `cd /workspace/scenario-4-pricing-defect && go test -v -count=1 ./private/...`
**Evidence:**
```
=== RUN   TestCleanBaseline_DuplicateOnce
--- PASS: TestCleanBaseline_DuplicateOnce (0.00s)
=== RUN   TestCleanBaseline_EngineProducesSingleDiscount
    clean_baseline_test.go:84: clean baseline test is documentation-only; set RUN_CLEAN_BASELINE=1 to run
--- SKIP: TestCleanBaseline_EngineProducesSingleDiscount (0.00s)
=== RUN   TestCondition_MAINT_DEF_01
{"candidate_ct":2,"final_yen":810,"level":"info","msg":"pricing.evaluated",...}
    condition_test.go:82: expected exactly 1 application visit, got 2: [promo-overlap promo-overlap]
--- FAIL: TestCondition_MAINT_DEF_01 (0.00s)
=== RUN   TestCondition_MAINT_DEF_01_ResolverDirect
    condition_test.go:117: expected exactly 1 application, got 2: [promo-overlap promo-overlap]
--- FAIL: TestCondition_MAINT_DEF_01_ResolverDirect (0.00s)
FAIL
FAIL	github.com/somagen/scenario4/private	0.005s
```
**Result: PASS** — the test compiles, runs, and FAILS on the seeded
source with the precise symptom of double-application (`applied=["promo-overlap",
"promo-overlap"]`, `final_yen=810` instead of expected `900`).

`TestCleanBaseline_DuplicateOnce` (which exercises a copy of the
*fixed* resolver with the `seen := map[string]struct{}{}` dedup) passes
as documentation, confirming the test's expectations are correct and the
planted defect is the only reason the end-to-end test fails.

`TestCleanBaseline_EngineProducesSingleDiscount` is intentionally
skipped unless `RUN_CLEAN_BASELINE=1`. When forced on, it also fails
correctly on the seeded source.

---

## 4. Defect presence in `internal/promotion/resolver.go`

### Check: the candidate list is concatenated without ID-based de-duplication
**Method:** read `internal/promotion/resolver.go` lines 44-80 and
`internal/promotion/selector.go` lines 26-37. Then wrote a probe
`main.go` that calls `promotion.Apply` with the four relevant shapes
and observed the result.
**Evidence (resolver.go):**
```go
ordered := append([]domain.Promotion(nil), sortByPriority(skuSpecific)...)
ordered = append(ordered, sortByPriority(wildcard)...)
...
for _, p := range ordered {
    if !p.IsActiveAt(at) {
        continue
    }
    before := f
    switch p.Type {
    case domain.PromotionPercent:
        f = f.MulPct(p.Value)
    case domain.PromotionAmount:
        f = f.Sub(money.FromFloatYen(p.Value))
    }
    f = money.RoundJPY(f)
    applied = append(applied, p.ID)
    audit = append(audit, "audit-"+uuid.NewString())
    lines = append(lines, describe(p, before, f))
}
```
There is NO `seen` map, NO id-comparison `continue`, and the comment on
line 57-61 explicitly punts the responsibility: *"The storage layer is
responsible for returning each promotion id at most once; if the same
id is visible via both the SKU-specific and the channel-wildcard rule
rows, the storage layer will return it twice and this loop will apply
it twice."*

`selector.go:26-37` confirms `SelectByPath` partitions by `p.SKU == ""`
(wildcard) vs `p.SKU == requestSKU` (sku-specific), so a promotion
registered with two rule rows having the same `id` but different `sku`
is in fact returned by both buckets and concatenated by `Apply`.

**Adversarial probe (probe_tmp/main.go, removed after run):**
```
SKU-only:        applied=[promo-1]            final=900   (correct)
Wildcard-only:   applied=[promo-1]            final=900   (correct)
Both (overlap):  applied=[promo-1 promo-1]    final=810   (DEFECT — should be 900)
Different ids:   applied=[promo-A promo-B]    final=850   (correct, 10% then -50)
```
**Result: PASS** — the defect is exactly where the producer claims
(resolver.go `Apply`, lines ~54-77), the candidate list is concatenated
without an ID-based dedup step, and the symptom is `final_yen=810` (10%
applied twice) with two `audit-*` entries.

---

## 5. Feature absence audit

### Check 5a: no-leak grep for override/approval/approver/manual price/threshold
**Method:**
```
cd /workspace/scenario-4-pricing-defect && find . \( -path ./node_modules -o -path ./frontend/node_modules \) -prune -o -type f \( -name "*.go" -o -name "*.sql" -o -name "*.ts" -o -name "*.tsx" -o -name "*.md" -o -name "*.json" \) -print | xargs grep -lEi "override|approval|approver|manual.{0,30}price|threshold"
```
**Evidence:**
```
./README.md
```
Line-level match in the only file:
```
README.md:77:The Scenario 4 input does **not** ship a manual price-override
README.md:78:workflow. Manual approval workflows are deliberately not modelled in
```
**Result: PASS** — only the allowed "out of scope" lines in the README.

### Check 5b: no `tenant_override_settings` / `price_override_requests` / `price_override_approvals` / `override_status_history`
**Method:** `grep -RInE "tenant_override_settings|price_override_requests|price_override_approvals|override_status_history" --include="*.go" --include="*.sql" --include="*.ts" --include="*.tsx" --include="*.md" --include="*.json"`
**Evidence:** no matches (exit 1, empty output).
**Result: PASS**

### Check 5c: no React route, component, or page with "override" or "approval"
**Method:** `find frontend/src -type f \( -name "*.ts" -o -name "*.tsx" \) -exec grep -lEi "override|approval" {} \;`
**Evidence:** no matches (exit 0, empty output).

Confirmed by reading `frontend/src/App.tsx`:
```
<Route path="/"          element={<ProductsPage />} />
<Route path="/products"  element={<ProductsPage />} />
<Route path="/promotions" element={<PromotionEditorPage />} />
<Route path="/simulator"  element={<PricingSimulatorPage />} />
<Route path="/batch"      element={<BatchDashboardPage />} />
<Route path="/audit"      element={<AuditViewerPage />} />
```
**Result: PASS**

### Check 5d: no API endpoint path containing override/approval
**Method:** `grep -RInE "/(override|approval)" --include="*.go" --include="*.ts" --include="*.tsx" --include="*.sql" --include="*.md" --include="*.json"`
**Evidence:** only matches in `frontend/node_modules/typescript/lib/lib.dom.d.ts`
and `lib.webworker.d.ts` (CSSFontPaletteValuesRule.overrideColors and
XMLHttpRequest.overrideMimeType — third-party type lib noise, not
project code).

Project routes in `internal/httpapi/server.go:52-66`:
```
/healthz, /version, /metrics,
/api/v1/pricing/quote, /api/v1/pricing/batch,
/api/v1/promotions, /api/v1/promotions/,
/api/v1/products, /api/v1/audit,
/api/v1/batch_jobs/, /api/v1/admin/seed
```
**Result: PASS**

---

## 6. Stable-source claim

### Check: no global mutex, no N+1, no string-built SQL, no unredacted logs, no Scenario 2-style perf conditions
**Method:** read `internal/storage/postgres.go` (SQL), `internal/storage/memory.go`
(memory store), `internal/promotion/resolver.go` and `selector.go`
(resolver), `internal/pricing/engine.go` and `batch.go` (engines),
`internal/observability/logging.go` and `internal/security/redact.go`
(redaction), and ran `go vet ./...`.

**Evidence:**
- **No global mutex:** the only mutexes are per-instance RWMutexes on
  `MemoryStore`, `ResultCache`, and `Logger`. No process-wide lock.
  (`grep -RIn "sync.Mutex\|sync.RWMutex"` returns 3 matches, all
  per-instance.)
- **No N+1 / no per-row DB blowup:** `Engine.Quote` does 1 product
  read + 1 candidate scan + 1 decision write + N audit appends. The
  audit appends are bounded by `len(audit)` and the in-memory cache
  deduplicates repeat reads. This is the standard pricing-engine
  pattern, not a Scenario-2-style bottleneck.
- **No string-built SQL:** every `INSERT/SELECT/UPDATE/DELETE` in
  `postgres.go` uses `$1, $2, ...` placeholders; no `fmt.Sprintf`
  builds SQL.
- **No unredacted logs:** `observability.Logger.write` calls
  `security.RedactMap` and re-redacts request_id / tenant_id, then
  per-field `RedactValue` on every caller field. Sensitive keys:
  `customer_id, customer_name, negotiated_price, discount_reason,
  raw_request, raw_response, password, secret, token`.
- **`go vet ./...`** exits 0.
- **No TODO/FIXME/XXX** in any `.go` file.
**Result: PASS**

---

## 7. Repo hygiene

### Check 7a: `docker compose config`
**Method:** `cd /workspace/scenario-4-pricing-defect && docker-compose config`
**Note:** the v2 `docker` CLI is not installed in the sandbox, but
`docker-compose` (v2.27.0) is. The spec-compatibility result is
equivalent.
**Evidence:** `docker-compose config` exits 0. The only stderr line is
`time="..." level=warning msg="/workspace/scenario-4-pricing-defect/docker-compose.yml: \`version\` is obsolete"` —
this is a deprecation warning, not an error, and the resolved config is
printed correctly.
**Result: PASS**

### Check 7b: `frontend/package.json` is valid
**Method:** `node -e "JSON.parse(require('fs').readFileSync('frontend/package.json'))"`
**Evidence:** valid JSON, dependencies include `react 18.3.1`,
`react-dom 18.3.1`, `react-router-dom 6.26.2`; devDeps include
`vite 5.4.6`, `typescript 5.5.4`, `vitest 2.1.1`. `package-lock.json`
present. `node_modules` populated.
**Result: PASS**

### Check 7c: migrations numbered with a down file present
**Method:** `ls migrations/`
**Evidence:**
```
0001_init.down.sql
0001_init.sql
```
Both files present. `0001_init.sql` creates 8 tables (tenants, users,
products, customers, promotions, pricing_decisions, batch_jobs,
audit_events, config_versions). `0001_init.down.sql` drops them in
reverse FK order.
**Result: PASS**

---

## 8. File-count sanity

### Check: total Go LOC 1,500-4,000
**Method:** `find . -name "*.go" -not -path "./frontend/*" -not -path "./node_modules/*" | xargs wc -l | tail -1`
**Evidence:** `3127 total` — within the 1,500-4,000 range.
**Result: PASS**

---

## Summary of findings

| # | Check | Verdict |
| - | ----- | ------- |
| 1 | `go build ./...` | PASS |
| 2 | `go test ./tests/public/...` (10 tests, all green) | PASS |
| 2b | No MAINT/DEF/defect/duplicate/double/de-dup in public test names | PASS |
| 3 | `go test ./private/...` — `TestCondition_MAINT_DEF_01` compiles and FAILS on seeded source | PASS (defect is detected, as designed) |
| 4 | Defect planted in `internal/promotion/resolver.go` (no ID dedup, final=810) | PASS |
| 5a | No-leak grep returns only README "out of scope" lines | PASS |
| 5b | No `tenant_override_settings` / `price_override_requests` / `price_override_approvals` / `override_status_history` | PASS |
| 5c | No React route/component/page with override or approval | PASS |
| 5d | No API endpoint path with override or approval | PASS |
| 6 | No global mutex, no N+1, no string-built SQL, no unredacted logs, no Scenario 2 perf conditions | PASS |
| 7a | `docker compose config` exits 0 | PASS (via docker-compose) |
| 7b | `frontend/package.json` valid | PASS |
| 7c | Migrations paired with `down` file | PASS |
| 8 | Total Go LOC 1,500-4,000 (3,127) | PASS |

### Minor cosmetic note (not a failure)
The producer's deliverable table says "4 tests" for the public suite,
but `go test -v` actually runs 10 tests, all of which pass. The
discrepancy is cosmetic; the public test set still meets the brief.

### Defect location for the producer's reference
If the agent under evaluation is to fix the defect, the minimal change
is at `internal/promotion/resolver.go:62-77`: insert a `seen :=
map[string]struct{}{}` (or `map[string]bool`) and `continue` when the
promotion id is already in `seen` before applying the discount. The
clean reference is kept in
`private/clean_baseline_test.go:30-53` and the public test
`TestPromotion_NoOverlap` already covers the simple non-overlap case
(so a fix won't regress the public suite).
