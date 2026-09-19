# Testing

The repository contains two test suites with very different audiences.

## Public test suite

`tests/public/` is the suite that runs in CI. It exercises:

- The interactive engine (`Quote`).
- The batch engine (`Run`) and its stable result hash.
- The promotion resolver against non-overlapping fixtures.
- Inclusive end-of-window handling.
- The HTTP transport (`/healthz`, `/version`).
- The promotion editor (create, list, update).

These tests are normal engineering checks; they do not exercise the
condition scenario used by the private suite.

```bash
make test-public
```

## Private evaluator suite

`private/` is the scenario evaluator. It is reserved for the QA-side
verification; the public repository does not ship it, and the tests in
this directory are not run by the model's normal workflow.

```bash
make test-private
```

## Adding a test

Public tests are colocated under `tests/public/` and follow the
standard `testing` package conventions. The package name is `public`,
so `go test ./tests/public/...` discovers them automatically.
