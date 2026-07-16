package v1

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Business metrics for shipping (RFC-0017 W2), answering the questions the order
// saga's on-call cares about:
//  1. Is CreateShipment failing the saga?      → created{outcome}
//  2. Is the compensation (cancel) failing?     → cancelled{outcome}
//  3. What is the shipment lookup hit/miss rate? → lookup{kind,found}
//
// Instruments ride the global OTel MeterProvider that obsx.SetupObservability
// installs (RFC-0014 OTLP pipeline → collector → VictoriaMetrics). Before that
// setup the global provider is a no-op, so package-init here is safe. Names are
// OTel-style; the collector renders them as shipment_created_total,
// shipment_cancelled_total, shipment_lookup_total.
//
// Labels are bounded to enumerable domain values (RFC-0017 D-9): no order ids,
// no tracking numbers, no free-form text.
var (
	meter = otel.Meter("shipping-service")

	shipmentCreatedCounter, _ = meter.Int64Counter("shipment.created.total",
		metric.WithDescription("CreateShipment outcomes (order-fulfillment saga step 2)"))
	shipmentCancelledCounter, _ = meter.Int64Counter("shipment.cancelled.total",
		metric.WithDescription("CancelShipment outcomes (saga compensation)"))
	shipmentLookupCounter, _ = meter.Int64Counter("shipment.lookup.total",
		metric.WithDescription("Shipment read lookups by kind and hit/miss"))
)

// Outcomes for the write paths (bounded).
const (
	outcomeOK           = "ok"
	outcomeInvalidOrder = "invalid_order_id"
	outcomeError        = "error"
)

// Lookup kinds (bounded).
const (
	lookupTrack   = "track"
	lookupByOrder = "by_order"
)

// recordShipmentCreated counts one CreateShipment call by outcome. Called once
// per call on each mutually exclusive return path: invalid_order_id (business
// rejection of a non-numeric id), error (repo/infra failure), or ok (a shipment
// now exists for the order). An idempotent replay returns ok — the repo's
// ON CONFLICT ... RETURNING hides first-insert-vs-existing from this layer, so
// ok reflects the real terminal outcome, not a distinct creation.
func recordShipmentCreated(ctx context.Context, outcome string) {
	shipmentCreatedCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}

// recordShipmentCancelled counts one CancelShipment call by outcome: error (repo
// failure) or ok (cancelled, including the idempotent no-op when there is no
// shipment or it is already cancelled).
func recordShipmentCancelled(ctx context.Context, outcome string) {
	shipmentCancelledCounter.Add(ctx, 1, metric.WithAttributes(attribute.String("outcome", outcome)))
}

// recordShipmentLookup counts one resolved read lookup by kind (track|by_order)
// and whether the shipment was found. Infra failures are deliberately NOT
// counted here — they are not a hit/miss signal and surface via the otelpgx DB
// span and pool error metrics — so found stays a clean boolean of existence.
func recordShipmentLookup(ctx context.Context, kind string, found bool) {
	shipmentLookupCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("kind", kind),
		attribute.Bool("found", found),
	))
}
