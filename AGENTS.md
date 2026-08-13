# shipping-service AGENTS guide

Instructions for AI agents and human contributors working in this repository.
Read it before making changes; keep edits surgical and consistent with what is
already here.

## Authority and scope

This repository implements the service. It does **not** define the contract.

- **Canonical contract:** [`homelab/docs/api/shipping.md`](https://github.com/duynhlab/homelab/blob/main/docs/api/shipping.md)
- **Shared API rules:** [`homelab/docs/api/api.md`](https://github.com/duynhlab/homelab/blob/main/docs/api/api.md)

Implement against those files. When this repository and the contract disagree,
**stop and classify the mismatch** using
[Resolving a mismatch](https://github.com/duynhlab/homelab/blob/main/docs/api/README.md#resolving-a-mismatch)
before changing either side. One class — an implementation that violates the
intended contract — **blocks the release tag**.

No route, RPC, payload or error inventory belongs in this file. Manifests,
gateway routing, NetworkPolicy and platform observability belong to
[duynhlab/homelab](https://github.com/duynhlab/homelab).

## Contribution workflow

- Never commit or push to `main`. Branch first, then open a PR.
- Branch names use conventional prefixes: `feat/`, `fix/`, `docs/`, `chore/`,
  `refactor/`, `test/`.
- Commit subjects: imperative mood, capitalised, ≤ 50 characters, no trailing
  period (`Add gRPC GetShipmentByOrder`, not `Added`/`Adds`). Add a body wrapped
  at 72 characters only when the change is non-trivial.
- Do not add attribution trailers (`Signed-off-by`, `Co-authored-by`,
  `Generated-by`, etc.), GitHub issue references, or `@`-mentions in commit
  messages. Put issue links in the PR description.
- PRs are squash-merged. CI (`go-check`) runs build, test, and lint on every PR
  and must be green before merge.

## Code quality

- Run `golangci-lint run` (v2+, `.golangci.yml`, 60+ linters) and fix every
  finding before committing. Common fixes:
  - `perfsprint`: prefer `errors.New` over `fmt.Errorf` when there are no verbs.
  - `nosprintfhostport`: use `net.JoinHostPort` over `fmt.Sprintf("%s:%s", …)`.
  - `errcheck`: check every error return, or explicitly `_ = fn()`.
  - `noctx`: use the `*WithContext` request constructors.
  - `goconst` / `gocognit`: extract repeated literals and split complex funcs.
- Keep changes idiomatic: dependency injection via constructor parameters,
  structured logging with `zap`, context propagation on all I/O.
- Write tests for new logic (see `internal/logic/v1/service_test.go`).
- Before pushing or opening a PR, verify Sonar new-code coverage ≥80%: run
  `go test -race -coverprofile=coverage.out ./...` and confirm changed lines are
  covered, including BOTH branches of any new conditional. `**/cmd/**`,
  `**/db/migrations/**`, `**/core/repository/**` are coverage-excluded;
  everything else counts.

## Build, test, lint

These are the commands CI runs, so a green local run means a green pipeline.

```bash
go build ./...
go vet ./...
go test -race ./...
go test -tags=integration ./internal/core/repository/...   # needs Docker (testcontainers)
golangci-lint run
```

Local development against an unreleased `pkg`: `pkg` is one module per package,
so its root has no `go.mod` and a single `replace github.com/duynhlab/pkg` can no
longer resolve. Use one commented `replace` line per module — the trailer in
`go.mod` shows the shape, and
[`docs/api/pkg.md`](https://github.com/duynhlab/homelab/blob/main/docs/api/pkg.md)
explains why.

## Architecture boundaries

**3-layer, dependencies flow one way only: transport → logic → core.**

- **Transport** — `internal/web/v1/` (HTTP) and `internal/grpc/v1/` (gRPC).
  Validate, map, delegate. Neither may touch the database or hold business rules.
- **Logic** — `internal/logic/v1/` holds the rules, including the quote rate
  table, and calls repository interfaces. No SQL, no transport types.
- **Core** — `internal/core/` owns the domain model, the repository interface and
  the Postgres implementation. It imports nothing from transport or logic.

Both transports always run. Observability is wired once through
`github.com/duynhlab/pkg/obsx`; the pool comes from `github.com/duynhlab/pkg/dbx`;
the gRPC server is built by `github.com/duynhlab/pkg/grpcx`; HTTP responses use the
shared `github.com/duynhlab/pkg/httpx` envelope.

## Invariants

Rules an implementer can violate at the keyboard.

- **Two money models, and they must never be mixed.** The **quote** is `int64`
  minor units. The **estimate** is `float64` dollars. They share no code path, and
  the estimate is an authority for nothing — it exists for display. Reusing one's
  arithmetic in the other silently changes what a customer is charged.
- **Idempotency lives in SQL, not the handler.** Creating a shipment is an upsert
  whose conflict branch is a no-op touch that still `RETURN`s the row, so a
  concurrent duplicate gets the existing shipment atomically. Replacing it with a
  select-then-insert reintroduces the race the unique constraint exists to close.
- **Compensation must succeed when there is nothing to cancel.** Cancelling a
  shipment that does not exist, or is already cancelled, affects zero rows and is
  still a success. A saga retries compensation; making it an error turns a
  successful rollback into a stuck workflow.
- **A missing shipment is not a gRPC error.** East-west, absence is an empty
  response; over HTTP the same absence is a 404. Do not "fix" the gRPC side into
  a `NotFound` status — a caller distinguishing absent from broken depends on it.
- **A non-numeric order id is rejected before Postgres.** Letting it reach the
  database raises a cast error, which surfaces as an internal failure rather than
  the clean "not found" or invalid-argument the caller should see.
- **`seed` is development-only** and refuses to run in production. It is invoked
  explicitly — never from `migrate` or the serve path — and it must not use
  golang-migrate: seeds are idempotent statements and must not share the
  `schema_migrations` version table.
- **Pooler-safe database settings live in `pkg/dbx`, not here.** Simple protocol
  and disabled statement caches are required by the pooler. The seed path
  re-asserts the execution mode explicitly because it runs multi-statement files.
  One DSN serves both the app and migrations; pool sizing is applied to the
  parsed config, never to the DSN, because the driver migrations use rejects
  `pool_*` parameters.
- **Graceful-shutdown ordering is load-bearing:** flag not-ready → readiness drain
  delay → HTTP shutdown → gRPC `GracefulStop` → pool close → OTel shutdown last,
  so pending spans, metrics and logs still flush.
- **Metric labels are bounded; no ids.** No order ids, no tracking numbers, no
  free-form text. An idempotent replay is deliberately counted as a success, and
  infrastructure failures are deliberately excluded from the lookup counter.
- **Probe suppression is one contract across logs and traces.** Successful
  `/health` and `/ready` requests are excluded from spans *and* access logs
  through the same skip list; a **failing** probe is still recorded. 4xx logs at
  warn, 5xx at error, and a trace id is only claimed when the span really has one.

## Repository map

- `cmd/main.go` — wiring, subcommand dispatch, HTTP + gRPC bootstrap, graceful shutdown
- `config/config.go` — env config, `Validate()`, `BuildDSN()`
- `db/migrations/` — forward-only golang-migrate SQL, embedded
- `db/seed/` — development-only demo seed, embedded
- `internal/web/v1/` — HTTP adapter
- `internal/grpc/v1/` — gRPC adapter and proto mapping
- `internal/logic/v1/` — service rules, the quote rate table, sentinel errors, metrics
- `internal/core/` — database wiring, domain model, repository interface and Postgres implementation
- `middleware/` — tracing and logging only

## Gotchas

- Kyverno admission rejects bad images. The published image is
  `ghcr.io/duynhlab/shipping-service/shipping-service:<tag>` — the repository path
  repeats, and the tag carries no `v` prefix. There is no separate migration
  image; the init container reuses the app image with `args: ["migrate"]`. Never
  `:latest`.
- Metrics leave over OTLP. There is no scrape endpoint and nothing scrapes this
  service.
- Migrations run against the direct database host, never the pooler — DDL through
  a transaction pooler is unsafe.

## API change synchronization

An API change is not done when the code compiles.

- The contract in homelab and this repository move **together** — same change,
  and either the same PR pair or an immediate follow-up.
- Behaviour that is designed but not deployed is marked **`Planned`** in the
  contract; it is never described as current.
- A material mismatch between the contract and this implementation **blocks the
  release tag** until it is reconciled or explicitly accepted.
