# shipping-service AGENTS guide

Instructions for AI agents and human contributors working in this repository.
Read it before making changes; keep edits surgical and consistent with what is
already here.

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

## Project overview

Shipping microservice for the `duynhlab` platform. Manages shipment tracking,
cost estimation, and delivery lookup. Go module
`github.com/duynhlab/shipping-service`. It serves an HTTP API on `:8080` and a
gRPC server (`shipping.v1.ShippingService`) on `:9090` consumed by
`order-service`.

## Repository layout

```
shipping-service/
├── cmd/main.go                       # Wires HTTP (:8080) + gRPC (:9090), graceful shutdown
├── config/config.go                  # Env-driven configuration + validation
├── db/migrations/                    # golang-migrate SQL (sql/000001_*.up.sql) + embed.go
├── internal/
│   ├── web/v1/handler.go             # HTTP handlers, JSON binding, DTO mapping
│   ├── logic/v1/                     # Business rules (service.go, errors.go, tests)
│   ├── core/
│   │   ├── database.go               # pgx/v5 pool (pooler-safe: simple protocol)
│   │   ├── domain/                   # Shipment model, repository interface, errors
│   │   └── repository/postgres/      # ShipmentRepository SQL implementation
│   └── grpc/v1/server.go             # shipping.v1.ShippingService server (adapter over logic)
└── middleware/                       # tracing, logging, prometheus, profiling, resource
```

## Build, test, lint

```bash
GOTOOLCHAIN=auto go build ./...   # compile (go.mod pins go 1.26.2)
GOTOOLCHAIN=auto go vet ./...     # vet
GOTOOLCHAIN=auto go test ./...    # tests
golangci-lint run                 # lint — must pass
```

## Conventions

- **3-layer architecture**, dependencies flow one way only: `web → logic →
  core`. Web handles HTTP/JSON/validation and delegates; Logic holds business
  rules and calls repository interfaces (no SQL, no `gin`, no
  `database.GetPool()`); Core owns domain models, the repository interface, and
  the Postgres implementation. Core imports nothing from Web or Logic.

  ```mermaid
  flowchart LR
      Web["web/v1<br/>HTTP handlers"] --> Logic["logic/v1<br/>business rules"]
      gRPC["grpc/v1<br/>transport adapter"] --> Logic
      Logic --> Core["core<br/>domain + repository"]
      Core --> DB[("PostgreSQL<br/>pgx/v5")]
  ```

- **gRPC SERVER**: this service exposes `shipping.v1.ShippingService` on `:9090`
  (`GRPC_PORT`). The only method is `GetShipmentByOrder`, which mirrors
  `GET /shipping/v1/internal/orders/:orderId` and is called by `order-service`
  on the order-details path. gRPC is the official east-west transport, so the
  server always runs; HTTP `:8080` is unaffected. Bootstrap via shared
  `github.com/duynhlab/pkg/grpcx` (`grpcx.NewServer` provides OpenTelemetry
  interceptors, health, reflection). Proto lives in
  `github.com/duynhlab/pkg/proto/shipping/v1`.
- **Observability** via shared `github.com/duynhlab/pkg/obsx`:
  - gRPC RED metrics (`rpc_server_*`) are exported through the global OTel
    MeterProvider onto the single `/metrics` handler (shared registry, scraped
    by the platform ServiceMonitor — no separate metrics port). `SetupMetrics()`
    runs before `grpcx.NewServer`.
  - Logging uses `obsx.TraceIDFromContext` so the log `trace_id` matches the
    active span.
  - HTTP middleware chain order is `tracing → logging → metrics`.
- **Diagrams**: Mermaid only. Never ASCII art.

## Gotchas

- The gRPC server (`internal/grpc/v1/server.go`) is a transport peer of the Web
  layer: it calls the same logic service and must never touch the database
  directly or embed business rules.
- A missing shipment is **not** an error. `GetShipmentByOrder` returns an empty
  response (unset shipment) when logic reports `ErrShipmentNotFound`, so callers
  treat "no shipment yet" like the HTTP 404 path.
- Kyverno admission rejects bad images: pin `ghcr.io/duynhlab/<service>:<sha>`
  or `:vX.Y.Z`, never `:latest`.
- Migrations run via golang-migrate v4.19.1, embedded through `db/migrations/embed.go`
  (`embed.FS`) and applied by `pkg/migratex` from the `migrate` subcommand. The init
  container reuses the app image (`args: ["migrate"]`) — no separate migration image.
  Migrations are forward-only `*.up.sql` files.
```
