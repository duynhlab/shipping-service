# shipping-service

Shipping microservice for tracking and cost estimation. Exposes an HTTP API
(browser + in-cluster) and a gRPC server used by `order-service` for
order-detail aggregation.

Module path: `github.com/duynhlab/shipping-service`.

## Features

- Shipment tracking
- Cost estimation
- Get shipment by order (HTTP `internal` + gRPC)

## HTTP API Endpoints

All routes follow Variant A naming — single path for browser and in-cluster callers. See [homelab naming convention](https://github.com/duynhlab/homelab/blob/main/docs/api/api-naming-convention.md).

| Method | Path | Audience |
|--------|------|----------|
| `GET` | `/shipping/v1/public/track` | public (query: `tracking_number`) |
| `GET` | `/shipping/v1/public/estimate` | public (query: `origin`, `destination`, `weight`) |
| `GET` | `/shipping/v1/internal/orders/:orderId` | internal (order-service aggregation; in-cluster only) |

Operational endpoints: `GET /health`, `GET /ready` (DB ping + drain-aware), `GET /metrics`.

## gRPC

shipping-service is a gRPC **server**. gRPC is the official east-west transport,
so the server always runs (HTTP `:8080` is unaffected).

- Listen address: `:9090` (`GRPC_PORT`, default `9090`)
- Service: `shipping.v1.ShippingService` (proto from `github.com/duynhlab/pkg/proto/shipping/v1`)
- Method: `GetShipmentByOrder` — mirrors `GET /shipping/v1/internal/orders/:orderId`; called by `order-service` on order-details
- Bootstrap via shared `github.com/duynhlab/pkg/grpcx` (`grpcx.NewServer`): OpenTelemetry interceptors, health, reflection
- A missing shipment returns an empty response (unset shipment), not an error

The gRPC server is a thin adapter (`internal/grpc/v1`) over the same logic layer
as the HTTP handlers, so both transports return identical data.

## Observability

- **Tracing**: OpenTelemetry (OTLP/HTTP to the collector). Middleware via `otelgin`.
- **Metrics**: Two sources on the same `/metrics` endpoint (shared registry, scraped by the platform ServiceMonitor — no separate port):
  - HTTP RED metrics (`request_duration_seconds`, `requests_in_flight`, request/response sizes) from the Prometheus middleware, with Tempo exemplars.
  - gRPC RED metrics (`rpc_server_*`) exported by `obsx.SetupMetrics()` (`github.com/duynhlab/pkg/obsx`) through the global OTel MeterProvider.
- **Logging**: structured Zap. The logging middleware uses `obsx.TraceIDFromContext` so log `trace_id` matches the active span (falls back to the request header / a generated ID).
- **Profiling**: Pyroscope (optional, `PROFILING_ENABLED`).

Gin middleware chain order: tracing → logging → metrics.

## Configuration

Configuration is loaded from environment variables (12-factor; `.env` is loaded
for local dev). Key variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `SERVICE_NAME` | _(required)_ | Service name (traces/profiles) |
| `PORT` | `8080` | HTTP listen port |
| `GRPC_PORT` | `9090` | gRPC listen port |
| `ENV` | `development` | `development`/`staging`/`production` |
| `TRACING_ENABLED` | `true` | Enable OTel tracing |
| `OTEL_COLLECTOR_ENDPOINT` | `otel-collector-opentelemetry-collector.monitoring.svc.cluster.local:4318` | OTLP/HTTP endpoint |
| `OTEL_SAMPLE_RATE` | `0.1` | Trace sample rate (0.0–1.0) |
| `METRICS_ENABLED` | `true` | Enable metrics setup |
| `PROFILING_ENABLED` | `true` | Enable Pyroscope profiling |
| `PYROSCOPE_ENDPOINT` | `http://pyroscope.monitoring.svc.cluster.local:4040` | Pyroscope endpoint |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `json` | Logging |
| `DB_HOST`, `DB_NAME`, `DB_USER`, `DB_PASSWORD` | _(required when `DB_HOST` set)_ | PostgreSQL connection |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_SSLMODE` | `disable` | SSL mode |
| `DB_POOL_MAX_CONNECTIONS` | `25` | Max pool connections |
| `SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown timeout |
| `READINESS_DRAIN_DELAY` | `5s` | Delay after failing readiness before HTTP shutdown |

## Tech Stack

- Go 1.26 + Gin
- gRPC (`google.golang.org/grpc`) via shared `pkg/grpcx`
- PostgreSQL via `pgx/v5` (simple protocol / no statement cache — pooler-safe)
- OpenTelemetry tracing, `pkg/obsx` metrics, Pyroscope profiling

## Development

### Prerequisites

- Go 1.26+
- [golangci-lint](https://golangci-lint.run/welcome/install/) v2+

### Local Development

```bash
# Install dependencies
go mod tidy
go mod download

# Build
go build ./...

# Test
go test ./...

# Lint (must pass before PR merge)
golangci-lint run --timeout=10m

# Run locally (requires .env or env vars)
go run cmd/main.go
```

### Pre-push Checklist

```bash
go build ./... && go test ./... && golangci-lint run --timeout=10m
```

## License

MIT
