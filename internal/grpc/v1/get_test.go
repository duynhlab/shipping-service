package v1

import (
	"context"
	"errors"
	"testing"

	shippingv1 "github.com/duynhlab/pkg/proto/shipping/v1"
	"github.com/duynhlab/shipping-service/internal/core/domain"
	logicv1 "github.com/duynhlab/shipping-service/internal/logic/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// getStub is a configurable ShipmentService double for GetShipmentByOrder.
type getStub struct {
	shipment *domain.Shipment
	err      error
}

func (s *getStub) GetShipmentByOrderID(_ context.Context, _ string) (*domain.Shipment, error) {
	return s.shipment, s.err
}
func (s *getStub) CreateShipment(_ context.Context, _ string) (*domain.Shipment, error) {
	return nil, nil
}
func (s *getStub) CancelShipment(_ context.Context, _ string) error { return nil }

func TestServer_GetShipmentByOrder(t *testing.T) {
	estimated := "2026-07-05T00:00:00Z"

	t.Run("success maps domain to proto", func(t *testing.T) {
		srv := NewServer(&getStub{shipment: &domain.Shipment{
			ID: 9, OrderID: 7, TrackingNumber: "MOP0000000007", Carrier: "MOP Express",
			Status: "pending", EstimatedDelivery: &estimated,
			CreatedAt: "2026-06-30T00:00:00Z", UpdatedAt: "2026-06-30T00:00:00Z",
		}})

		resp, err := srv.GetShipmentByOrder(context.Background(), &shippingv1.GetShipmentByOrderRequest{OrderId: "7"})
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		got := resp.GetShipment()
		if got.GetId() != "9" || got.GetOrderId() != "7" {
			t.Errorf("id/order = %q/%q, want 9/7", got.GetId(), got.GetOrderId())
		}
		if got.GetTrackingNumber() != "MOP0000000007" {
			t.Errorf("tracking = %q, want MOP0000000007", got.GetTrackingNumber())
		}
		if got.GetEstimatedDelivery() != estimated {
			t.Errorf("estimated_delivery = %q, want %q", got.GetEstimatedDelivery(), estimated)
		}
	})

	t.Run("not found returns empty response, no error", func(t *testing.T) {
		srv := NewServer(&getStub{err: logicv1.ErrShipmentNotFound})
		resp, err := srv.GetShipmentByOrder(context.Background(), &shippingv1.GetShipmentByOrderRequest{OrderId: "404"})
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if resp.GetShipment() != nil {
			t.Errorf("shipment = %v, want nil (empty response)", resp.GetShipment())
		}
	})

	t.Run("nil shipment returns empty response", func(t *testing.T) {
		srv := NewServer(&getStub{shipment: nil, err: nil})
		resp, err := srv.GetShipmentByOrder(context.Background(), &shippingv1.GetShipmentByOrderRequest{OrderId: "7"})
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if resp.GetShipment() != nil {
			t.Errorf("shipment = %v, want nil", resp.GetShipment())
		}
	})

	t.Run("generic error -> Internal", func(t *testing.T) {
		srv := NewServer(&getStub{err: errors.New("db down")})
		_, err := srv.GetShipmentByOrder(context.Background(), &shippingv1.GetShipmentByOrderRequest{OrderId: "7"})
		if status.Code(err) != codes.Internal {
			t.Fatalf("got code %v, want Internal", status.Code(err))
		}
	})

	t.Run("toProto without estimated delivery leaves it empty", func(t *testing.T) {
		srv := NewServer(&getStub{shipment: &domain.Shipment{ID: 1, OrderID: 2, TrackingNumber: "MOP0000000002", Status: "pending"}})
		resp, err := srv.GetShipmentByOrder(context.Background(), &shippingv1.GetShipmentByOrderRequest{OrderId: "2"})
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if resp.GetShipment().GetEstimatedDelivery() != "" {
			t.Errorf("estimated_delivery = %q, want empty", resp.GetShipment().GetEstimatedDelivery())
		}
	})
}
