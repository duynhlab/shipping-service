# shipping-service

Shipping microservice for tracking and cost estimation.

## Features

- Shipment tracking
- Cost estimation
- Get shipment by order

## API Endpoints

> **Browser callers** hit `https://gateway.duynhne.me/shipping/v1/public/{track,estimate}`; Kong rewrites to the cluster paths below. `GET /api/v1/shipping/orders/:id` stays internal — called only by `order-service` for aggregation. See [homelab naming convention](https://github.com/duynhlab/homelab/blob/main/docs/api/api-naming-convention.md).

| Method | Cluster path | Edge path (via gateway) |
|--------|--------------|-------------------------|
| `GET` | `/api/v1/shipping/track` | `/shipping/v1/public/track` |
| `GET` | `/api/v1/shipping/estimate` | `/shipping/v1/public/estimate` |
| `GET` | `/api/v1/shipping/orders/:id` | *(internal — not on gateway)* |

## Tech Stack

- Go + Gin framework
- PostgreSQL 16 (supporting-db cluster, cross-namespace)
- PgBouncer connection pooling
- OpenTelemetry tracing

## Development

### Prerequisites

- Go 1.25+
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
