package v1

import (
	"context"
	"errors"
	"testing"

	"github.com/duynhlab/shipping-service/internal/core/domain"
)

func TestCreateShipment(t *testing.T) {
	t.Run("returns the created shipment", func(t *testing.T) {
		want := &domain.Shipment{ID: 9, OrderID: 7, TrackingNumber: "MOP0000000007", Status: "pending"}
		repo := &mockShipmentRepository{createResult: want}
		svc := NewShippingService(repo)

		got, err := svc.CreateShipment(context.Background(), "7")
		if err != nil {
			t.Fatalf("CreateShipment returned %v, want nil", err)
		}
		if got != want {
			t.Errorf("got shipment %+v, want %+v", got, want)
		}
		if repo.createdID != "7" {
			t.Errorf("repo got orderID %q, want 7", repo.createdID)
		}
	})

	t.Run("propagates repo errors", func(t *testing.T) {
		repo := &mockShipmentRepository{createErr: errors.New("db down")}
		svc := NewShippingService(repo)

		if _, err := svc.CreateShipment(context.Background(), "7"); err == nil {
			t.Fatal("CreateShipment returned nil, want an error")
		}
	})

	t.Run("rejects a non-numeric order id before hitting the repo", func(t *testing.T) {
		repo := &mockShipmentRepository{createResult: &domain.Shipment{ID: 1}}
		svc := NewShippingService(repo)

		_, err := svc.CreateShipment(context.Background(), "not-a-number")
		if !errors.Is(err, ErrInvalidOrderID) {
			t.Fatalf("CreateShipment(non-numeric) err = %v, want ErrInvalidOrderID", err)
		}
		if repo.createdID != "" {
			t.Errorf("repo was called with %q, want no call", repo.createdID)
		}
	})
}

func TestCancelShipment(t *testing.T) {
	t.Run("cancels via the repo", func(t *testing.T) {
		repo := &mockShipmentRepository{}
		svc := NewShippingService(repo)

		if err := svc.CancelShipment(context.Background(), "7"); err != nil {
			t.Fatalf("CancelShipment returned %v, want nil", err)
		}
		if repo.cancelledID != "7" {
			t.Errorf("repo got orderID %q, want 7", repo.cancelledID)
		}
	})

	t.Run("propagates repo errors", func(t *testing.T) {
		repo := &mockShipmentRepository{cancelErr: errors.New("db down")}
		svc := NewShippingService(repo)

		if err := svc.CancelShipment(context.Background(), "7"); err == nil {
			t.Fatal("CancelShipment returned nil, want an error")
		}
	})
}
