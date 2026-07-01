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

type stubShipmentSvc struct {
	created    *domain.Shipment
	createErr  error
	cancelErr  error
	gotOrderID string
}

func (s *stubShipmentSvc) GetShipmentByOrderID(_ context.Context, _ string) (*domain.Shipment, error) {
	return nil, nil
}

func (s *stubShipmentSvc) CreateShipment(_ context.Context, orderID string) (*domain.Shipment, error) {
	s.gotOrderID = orderID
	return s.created, s.createErr
}

func (s *stubShipmentSvc) CancelShipment(_ context.Context, orderID string) error {
	s.gotOrderID = orderID
	return s.cancelErr
}

func TestServer_CreateShipment(t *testing.T) {
	t.Run("success returns the shipment", func(t *testing.T) {
		stub := &stubShipmentSvc{created: &domain.Shipment{ID: 9, OrderID: 7, TrackingNumber: "MOP0000000007", Status: "pending"}}
		srv := NewServer(stub)

		resp, err := srv.CreateShipment(context.Background(), &shippingv1.CreateShipmentRequest{OrderId: "7", Address: "1 Main St"})
		if err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if resp.GetShipment().GetTrackingNumber() != "MOP0000000007" {
			t.Errorf("tracking = %q, want MOP0000000007", resp.GetShipment().GetTrackingNumber())
		}
		if stub.gotOrderID != "7" {
			t.Errorf("svc got orderID %q, want 7", stub.gotOrderID)
		}
	})

	t.Run("missing order_id -> InvalidArgument", func(t *testing.T) {
		srv := NewServer(&stubShipmentSvc{})
		_, err := srv.CreateShipment(context.Background(), &shippingv1.CreateShipmentRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("invalid order id -> InvalidArgument", func(t *testing.T) {
		srv := NewServer(&stubShipmentSvc{createErr: logicv1.ErrInvalidOrderID})
		_, err := srv.CreateShipment(context.Background(), &shippingv1.CreateShipmentRequest{OrderId: "not-a-number"})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("repo error -> Internal", func(t *testing.T) {
		srv := NewServer(&stubShipmentSvc{createErr: errors.New("db down")})
		_, err := srv.CreateShipment(context.Background(), &shippingv1.CreateShipmentRequest{OrderId: "7"})
		if status.Code(err) != codes.Internal {
			t.Fatalf("got code %v, want Internal", status.Code(err))
		}
	})
}

func TestServer_CancelShipment(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		stub := &stubShipmentSvc{}
		srv := NewServer(stub)
		if _, err := srv.CancelShipment(context.Background(), &shippingv1.CancelShipmentRequest{OrderId: "7"}); err != nil {
			t.Fatalf("got error %v, want nil", err)
		}
		if stub.gotOrderID != "7" {
			t.Errorf("svc got orderID %q, want 7", stub.gotOrderID)
		}
	})

	t.Run("missing order_id -> InvalidArgument", func(t *testing.T) {
		srv := NewServer(&stubShipmentSvc{})
		_, err := srv.CancelShipment(context.Background(), &shippingv1.CancelShipmentRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("got code %v, want InvalidArgument", status.Code(err))
		}
	})

	t.Run("repo error -> Internal", func(t *testing.T) {
		srv := NewServer(&stubShipmentSvc{cancelErr: errors.New("db down")})
		_, err := srv.CancelShipment(context.Background(), &shippingv1.CancelShipmentRequest{OrderId: "7"})
		if status.Code(err) != codes.Internal {
			t.Fatalf("got code %v, want Internal", status.Code(err))
		}
	})
}
