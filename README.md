# shipping-service

Shipments and shipping quotes: one shipment row per order, its tracking number
and status, and the rate table the checkout total is built from.

## Responsibilities

- **Owns:** shipments, tracking numbers, shipment status, and the quote rate
  table (static, in code).
- **Does not own:** orders (`order-service`), payments (`payment-service`),
  carrier integration, or customer addresses — an address is accepted on a
  create request but never persisted. This service dials no other service.

## Tech

| Area | Technology |
|------|------------|
| Runtime | Go 1.26 |
| Transports | HTTP (public tracking and estimate) · gRPC (east-west) |
| Data | PostgreSQL — one table, `shipments` |
| Platform libraries | `dbx`, `grpcx`, `httpx`, `logger/zapx`, `migratex`, `obsx`, `proto` |

## API

- **Canonical contract:** [`homelab/docs/api/shipping.md`](https://github.com/duynhlab/homelab/blob/main/docs/api/shipping.md)
- **Shared conventions:** [`homelab/docs/api/api.md`](https://github.com/duynhlab/homelab/blob/main/docs/api/api.md)
- **Surfaces:** public HTTP for tracking and a delivery estimate, plus
  `shipping.v1.ShippingService` east-west — checkout asks it for a quote, and
  the order saga creates and cancels shipments through it. HTTP `:8080` also
  carries `/health` and `/ready`.

Routes, RPC semantics, payloads and error reasons live in the contract, so there
is one place to change when they change.

## Run locally

Prefer the homelab **local-stack** — a quote is only meaningful with a checkout
in front of it, and a shipment with an order behind it.

Standalone you need PostgreSQL reachable through the `DB_*` variables:

```bash
go run cmd/main.go migrate   # apply schema migrations
go run cmd/main.go seed      # demo shipments — development only, refuses production
go run cmd/main.go           # serve HTTP :8080 + gRPC :9090
```

## Verify

The commands CI runs, so a green local run means a green pipeline:

```bash
go build ./...
go test -race ./...
go test -tags=integration ./internal/core/repository/...   # needs Docker (testcontainers)
golangci-lint run
```

## Docs

- [Canonical contract](https://github.com/duynhlab/homelab/blob/main/docs/api/shipping.md)
- [local-stack guide](https://github.com/duynhlab/homelab/blob/main/local-stack/README.md)

## License

MIT
