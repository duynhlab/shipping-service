package v1

import (
	"context"
	"errors"
	"testing"

	"github.com/duynhlab/shipping-service/internal/core/domain"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// collectByLabel reads counter name into an attribute→value map keyed by one
// label.
func collectByLabel(t *testing.T, reader sdkmetric.Reader, name, label string) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	forEachDataPoint(t, reader, name, func(attrs attribute.Set, v int64) {
		key, _ := attrs.Value(attribute.Key(label))
		out[key.AsString()] = v
	})
	return out
}

// collectLookup reads shipment.lookup.total into a "kind/found" → value map so
// both bounded labels can be asserted together.
func collectLookup(t *testing.T, reader sdkmetric.Reader) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	forEachDataPoint(t, reader, "shipment.lookup.total", func(attrs attribute.Set, v int64) {
		kind, _ := attrs.Value("kind")
		found, _ := attrs.Value("found")
		out[kind.AsString()+"/"+found.String()] = v
	})
	return out
}

// assertCounts collects counter name keyed by label and checks each want.
func assertCounts(t *testing.T, reader sdkmetric.Reader, name, label string, want map[string]int64) {
	t.Helper()
	got := collectByLabel(t, reader, name, label)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s{%s=%s} = %d, want %d", name, label, k, got[k], v)
		}
	}
}

func forEachDataPoint(t *testing.T, reader sdkmetric.Reader, name string, fn func(attribute.Set, int64)) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s is %T, want Sum[int64]", name, m.Data)
			}
			for _, dp := range sum.DataPoints {
				fn(dp.Attributes, dp.Value)
			}
		}
	}
}

// TestShippingBusinessMetrics drives every outcome/label of the three W2
// counters on one MeterProvider. The OTel global meter delegate is first-wins,
// so the binary installs exactly one provider here and asserts the cumulative
// counters after each group. Every recorder fires exactly once per logic call,
// so the totals below are the number of calls that took each branch.
func TestShippingBusinessMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	ctx := context.Background()

	// --- shipment.created.total {outcome} ---
	okSvc := NewShippingService(&mockShipmentRepository{createResult: &domain.Shipment{ID: 1, OrderID: 7}})
	if _, err := okSvc.CreateShipment(ctx, "7"); err != nil {
		t.Fatalf("create ok: %v", err)
	}
	// Idempotent replay: the repo returns the existing shipment again. It is a
	// real successful outcome (indistinguishable from a first insert at this
	// layer), so it counts as another ok — not a separate replay label.
	if _, err := okSvc.CreateShipment(ctx, "7"); err != nil {
		t.Fatalf("create replay: %v", err)
	}
	errSvc := NewShippingService(&mockShipmentRepository{createErr: errors.New("db down")})
	if _, err := errSvc.CreateShipment(ctx, "7"); err == nil {
		t.Fatal("create error: want error")
	}
	if _, err := okSvc.CreateShipment(ctx, "not-a-number"); !errors.Is(err, ErrInvalidOrderID) {
		t.Fatalf("create invalid: err = %v, want ErrInvalidOrderID", err)
	}

	assertCounts(t, reader, "shipment.created.total", "outcome",
		map[string]int64{"ok": 2, "error": 1, "invalid_order_id": 1})

	// --- shipment.cancelled.total {outcome} ---
	if err := okSvc.CancelShipment(ctx, "7"); err != nil {
		t.Fatalf("cancel ok: %v", err)
	}
	cancelErrSvc := NewShippingService(&mockShipmentRepository{cancelErr: errors.New("db down")})
	if err := cancelErrSvc.CancelShipment(ctx, "7"); err == nil {
		t.Fatal("cancel error: want error")
	}
	assertCounts(t, reader, "shipment.cancelled.total", "outcome",
		map[string]int64{"ok": 1, "error": 1})

	// --- shipment.lookup.total {kind,found} ---
	found := &domain.Shipment{ID: 2, OrderID: 10, TrackingNumber: "TRK", Status: "delivered"}
	// track
	if _, err := NewShippingService(&mockShipmentRepository{shipment: found}).TrackShipment(ctx, "TRK"); err != nil {
		t.Fatalf("track found: %v", err)
	}
	if _, err := NewShippingService(&mockShipmentRepository{err: domain.ErrShipmentNotFound}).TrackShipment(ctx, "X"); !errors.Is(err, ErrShipmentNotFound) {
		t.Fatalf("track missing: %v", err)
	}
	// infra error must NOT be counted on the business lookup counter
	if _, err := NewShippingService(&mockShipmentRepository{err: errors.New("db down")}).TrackShipment(ctx, "X"); err == nil {
		t.Fatal("track infra: want error")
	}
	// by_order
	if _, err := NewShippingService(&mockShipmentRepository{shipment: found}).GetShipmentByOrderID(ctx, "10"); err != nil {
		t.Fatalf("by_order found: %v", err)
	}
	if _, err := NewShippingService(&mockShipmentRepository{err: domain.ErrShipmentNotFound}).GetShipmentByOrderID(ctx, "404"); !errors.Is(err, ErrShipmentNotFound) {
		t.Fatalf("by_order missing: %v", err)
	}
	// non-numeric order id resolves to not-found before the repo
	if _, err := NewShippingService(&mockShipmentRepository{}).GetShipmentByOrderID(ctx, "not-a-number"); !errors.Is(err, ErrShipmentNotFound) {
		t.Fatalf("by_order non-numeric: %v", err)
	}
	if _, err := NewShippingService(&mockShipmentRepository{err: errors.New("db down")}).GetShipmentByOrderID(ctx, "500"); err == nil {
		t.Fatal("by_order infra: want error")
	}

	lookup := collectLookup(t, reader)
	assertLookupCounts(t, lookup)
}

// assertLookupCounts checks the four bounded {kind,found} cells and that infra
// errors added nothing (exactly 5 counted lookups).
func assertLookupCounts(t *testing.T, lookup map[string]int64) {
	t.Helper()
	want := map[string]int64{
		"track/true":     1,
		"track/false":    1,
		"by_order/true":  1,
		"by_order/false": 2,
	}
	var total int64
	for key, w := range want {
		if lookup[key] != w {
			t.Errorf("shipment.lookup.total{%s} = %d, want %d", key, lookup[key], w)
		}
	}
	for _, v := range lookup {
		total += v
	}
	if total != 5 {
		t.Errorf("shipment.lookup.total sum = %d, want 5 (infra errors must not be counted)", total)
	}
}
