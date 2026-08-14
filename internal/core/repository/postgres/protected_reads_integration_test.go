//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/duynhlab/shipping-service/internal/core/domain"
)

// TestProtectedReads_Integration proves the operator projections over the
// real schema (RFC-0023 slice A): list paging + status filter and the
// by-id case view.
func TestProtectedReads_Integration(t *testing.T) {
	pool := newTestDB(t)
	repo := NewShipmentRepository(pool)
	ctx := context.Background()

	// Seed through the real write path (idempotent by order id).
	a, err := repo.CreateShipment(ctx, "9001")
	if err != nil {
		t.Fatalf("seed shipment a: %v", err)
	}
	if _, err := repo.CreateShipment(ctx, "9002"); err != nil {
		t.Fatalf("seed shipment b: %v", err)
	}
	if err := repo.CancelShipment(ctx, "9002"); err != nil {
		t.Fatalf("cancel shipment b: %v", err)
	}

	items, total, err := repo.ListShipments(ctx, "", 10, 0)
	if err != nil || total < 2 || len(items) < 2 {
		t.Fatalf("list = (%d items, total %d, %v), want >=2", len(items), total, err)
	}
	// Newest first: shipment b (higher id) leads.
	if items[0].OrderID != 9002 {
		t.Fatalf("order: want newest (9002) first, got %+v", items[0])
	}

	cancelled, total, err := repo.ListShipments(ctx, "cancelled", 10, 0)
	if err != nil || total != 1 || cancelled[0].OrderID != 9002 {
		t.Fatalf("cancelled filter = (%v, total %d, %v)", cancelled, total, err)
	}

	page2, _, err := repo.ListShipments(ctx, "", 1, 1)
	if err != nil || len(page2) != 1 || page2[0].ID == items[0].ID {
		t.Fatalf("paging broken: %v %v", page2, err)
	}

	got, err := repo.GetByID(ctx, a.ID)
	if err != nil || got.OrderID != 9001 || got.TrackingNumber == "" {
		t.Fatalf("get by id = %+v, %v", got, err)
	}
	if _, err := repo.GetByID(ctx, 999999); !errors.Is(err, domain.ErrShipmentNotFound) {
		t.Fatalf("missing id: want ErrShipmentNotFound, got %v", err)
	}
}
