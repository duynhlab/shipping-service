# shipping-service

> AI Agent context for understanding this repository

## 📋 Overview

Shipping microservice. Manages shipment tracking, cost estimation, and delivery.

## 🏗️ Architecture Guidelines

### 3-Layer Architecture

| Layer | Location | Responsibility |
|-------|----------|----------------|
| **Web** | `internal/web/v1/handler.go` | HTTP, validation |
| **Logic** | `internal/logic/v1/service.go` | Business rules (❌ NO SQL) |
| **Core** | `internal/core/` | Domain models, repositories |

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
├── cmd/main.go
├── config/config.go
├── db/migrations/sql/
├── internal/
│   ├── core/
│   │   ├── database.go
│   │   └── domain/
│   ├── logic/v1/service.go
│   └── web/v1/handler.go
├── middleware/
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
| Framework | Gin |
| Database | PostgreSQL 16 via pgx/v5 |
| Tracing | OpenTelemetry |

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

### Cluster paths (what this service mounts)

| Method | Cluster path | Audience | Description |
|--------|--------------|----------|-------------|
| `GET` | `/api/v1/shipping/track` | public | Track shipment (query: `tracking_number`) |
| `GET` | `/api/v1/shipping/estimate` | public | Estimate shipping cost |
| `GET` | `/api/v1/shipping/orders/:orderId` | internal | Get shipment by order ID — called by `order-service`, **not on gateway** |

### Edge paths (what the browser sends)

Kong in the `shipping` namespace rewrites `/shipping/v1/public/{verb}` → `/api/v1/shipping/{verb}`. The internal `orders/:orderId` route stays off the gateway; `order-service` calls it via `http://shipping.shipping.svc.cluster.local:8080/api/v1/shipping/orders/:orderId`.

| Edge path (browser) | → Cluster path |
|---------------------|----------------|
| `GET gateway.duynhne.me/shipping/v1/public/track?tracking_number=…` | `GET /api/v1/shipping/track` |
| `GET gateway.duynhne.me/shipping/v1/public/estimate?…` | `GET /api/v1/shipping/estimate` |
| *(no edge path)* | `GET /api/v1/shipping/orders/:orderId` — internal only |

Convention + rewrite rule: [`homelab/docs/api/api-naming-convention.md`](https://github.com/duynhlab/homelab/blob/main/docs/api/api-naming-convention.md).
