# shipping-service

> AI Agent context for understanding this repository

## 📋 Overview

Shipping microservice. Manages shipment tracking, cost estimation, and delivery.

Module path: `github.com/duynhlab/shipping-service`. It serves an HTTP API and a
gRPC server (`shipping.v1.ShippingService`) consumed by `order-service`.

## 🏗️ Architecture Guidelines

### 3-Layer Architecture

| Layer | Location | Responsibility |
|-------|----------|----------------|
| **Web** | `internal/web/v1/handler.go` | HTTP, validation |
| **gRPC** | `internal/grpc/v1/server.go` | gRPC transport adapter (mirrors Web) |
| **Logic** | `internal/logic/v1/service.go` | Business rules (❌ NO SQL) |
| **Core** | `internal/core/` | Domain models, repository interface + Postgres impl |

The gRPC server is a thin adapter over the **same** logic layer as the HTTP
handlers, so both transports return identical data. It lives at the transport
level (alongside Web) and must never embed business rules.

### 3-Layer Coding Rules

**CRITICAL**: Strict layer boundaries. Violations will be rejected in code review.

#### Layer Boundaries

| Layer | Location | ALLOWED | FORBIDDEN |
|-------|----------|---------|-----------|
| **Web** | `internal/web/v1/` | HTTP handling, JSON binding, DTO mapping, call Logic, aggregation | SQL queries, direct DB access, business rules |
| **Logic** | `internal/logic/v1/` | Business rules, call repository interfaces, domain errors | SQL queries, `database.GetPool()`, HTTP handling, `*gin.Context` |
| **Core** | `internal/core/` | Domain models, repository implementations, SQL queries, DB connection | HTTP handling, business orchestration |

#### Dependency Direction

```
Web -> Logic -> Core (one-way only, never reverse)
```

- Web imports Logic and Core/domain
- Logic imports Core/domain and Core/repository interfaces
- Core imports nothing from Web or Logic

#### DO

- Put HTTP handlers, request validation, error-to-status mapping in `web/`
- Put business rules, orchestration, transaction logic in `logic/`
- Put SQL queries in `core/repository/` implementations
- Use repository interfaces (defined in `core/domain/`) for data access in Logic layer
- Use dependency injection (constructor parameters) for all service dependencies

#### DO NOT

- Write SQL or call `database.GetPool()` in Logic layer
- Import `gin` or handle HTTP in Logic layer
- Put business rules in Web layer (Web only translates and delegates)
- Call Logic functions directly from another service (use HTTP aggregation in Web layer)
- Skip the Logic layer (Web must not call Core/repository directly)

### Directory Structure

```
shipping-service/
├── cmd/main.go              # Wires HTTP (:8080) + gRPC (:9090) servers
├── config/config.go
├── db/migrations/sql/
├── internal/
│   ├── core/
│   │   ├── database.go      # pgx/v5 pool (pooler-safe: simple protocol)
│   │   ├── domain/          # Shipment model, repository interface, errors
│   │   └── repository/postgres/  # ShipmentRepository impl (SQL)
│   ├── logic/v1/service.go
│   ├── grpc/v1/server.go    # shipping.v1.ShippingService server (adapter over logic)
│   └── web/v1/handler.go
├── middleware/              # tracing, logging, prometheus, profiling, resource
└── Dockerfile
```

## 🛠️ Development Workflow

### Code Quality

**MANDATORY**: All code changes MUST pass lint before committing.

- Linter: `golangci-lint` v2+ with `.golangci.yml` config (60+ linters enabled)
- Zero tolerance: PRs with lint errors will NOT be merged
- CI enforces: `go-check` job runs lint on every PR

#### Commands (run in order)

```bash
go mod tidy              # Clean dependencies
go build ./...           # Verify compilation
go test ./...            # Run tests
golangci-lint run --timeout=10m  # Lint (MUST pass)
```

#### Pre-commit One-liner

```bash
go build ./... && go test ./... && golangci-lint run --timeout=10m
```

### Common Lint Fixes

- `perfsprint`: Use `errors.New()` instead of `fmt.Errorf()` when no format verbs
- `nosprintfhostport`: Use `net.JoinHostPort()` instead of `fmt.Sprintf("%s:%s", host, port)`
- `errcheck`: Always check error returns (or explicitly `_ = fn()`)
- `goconst`: Extract repeated string literals to constants
- `gocognit`: Extract helper functions to reduce complexity
- `noctx`: Use `http.NewRequestWithContext()` instead of `http.NewRequest()`

## 🔧 Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.26 |
| HTTP Framework | Gin |
| gRPC | `google.golang.org/grpc` via shared `github.com/duynhlab/pkg/grpcx` |
| Database | PostgreSQL via `pgx/v5` (simple protocol, pooler-safe) |
| Tracing | OpenTelemetry (`otelgin`, OTLP/HTTP) |
| Metrics | `github.com/duynhlab/pkg/obsx` + Prometheus middleware |
| Profiling | Pyroscope |

## 📡 gRPC (east-west transport)

shipping-service is a gRPC **server**. gRPC is the official east-west transport,
so the server **always runs** on `:9090` (`GRPC_PORT`); HTTP `:8080` is unaffected.
It returns `nil` only if the listener cannot bind.

- Proto: `github.com/duynhlab/pkg/proto/shipping/v1`
- Service: `shipping.v1.ShippingService`
- Method: `GetShipmentByOrder` — mirrors `GET /shipping/v1/internal/orders/:orderId`; called by `order-service` on order-details
- Bootstrap: `grpcx.NewServer()` provides OpenTelemetry interceptors, health, reflection
- Missing shipment → empty response (unset shipment), **not** an error — callers treat "no shipment yet" like the HTTP 404 path

## 📈 Observability

- **Metrics on a single `/metrics`** (shared registry, scraped by the platform
  ServiceMonitor — **no separate metrics port**):
  - HTTP RED metrics (`request_duration_seconds`, `requests_in_flight`, sizes)
    from `middleware/prometheus.go`, with Tempo exemplars.
  - gRPC RED metrics (`rpc_server_*`) from `obsx.SetupMetrics()` via the global
    OTel MeterProvider. `SetupMetrics()` runs before `grpcx.NewServer`.
- **Logging**: the logging middleware uses `obsx.TraceIDFromContext` so the log
  `trace_id` matches the active span (falls back to header/generated ID).
- **Middleware chain order**: tracing → logging → metrics.

## 🏗️ Infrastructure Details

### Database

| Component | Value |
|-----------|-------|
| **Cluster** | supporting-db (shared with user, notification) |
| **PostgreSQL** | 16 |
| **HA** | Single instance |
| **Pooler** | PgBouncer Sidecar |
| **Endpoint** | `supporting-db-pooler.user.svc.cluster.local:5432` |
| **Pool Mode** | Transaction |
| **Cross-namespace** | Yes (cluster in `user` namespace) |

**Note:** Database cluster is in `user` namespace. Zalando Operator syncs credentials via cross-namespace secret.

### Graceful Shutdown

**VictoriaMetrics Pattern:**
1. `/ready` → 503 when shutting down
2. Drain delay (5s)
3. Sequential: HTTP → Database → Tracer

## 🔌 API Reference

Routes are mounted directly at `/{service}/v1/{audience}/…` (Variant A — single URL shape). Kong is pure pass-through for `public`; `internal` is reachable only via service DNS.

| Method | Path | Audience | Description |
|--------|------|----------|-------------|
| `GET` | `/shipping/v1/public/track` | public | Track shipment (query: `tracking_number`) |
| `GET` | `/shipping/v1/public/estimate` | public | Estimate shipping cost |
| `GET` | `/shipping/v1/internal/orders/:orderId` | internal | Get shipment by order ID — HTTP fallback; primary transport is gRPC `shipping.v1.ShippingService/GetShipmentByOrder` on `:9090`, called by `order-service` |

Full convention + inventory: [`homelab/docs/api/api-naming-convention.md`](https://github.com/duynhlab/homelab/blob/main/docs/api/api-naming-convention.md).
