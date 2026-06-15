// Package v1 implements the gRPC transport for shipping, version 1. It is a
// thin adapter over the logic layer (mirroring internal/web/v1) so the gRPC and
// HTTP paths share the same business logic and return identical data.
package v1

import (
	"context"
	"errors"
	"strconv"

	shippingv1 "github.com/duynhlab/pkg/proto/shipping/v1"
	"github.com/duynhlab/shipping-service/internal/core/domain"
	logicv1 "github.com/duynhlab/shipping-service/internal/logic/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ShipmentService is the logic-layer dependency the gRPC server needs.
// *logicv1.ShippingService satisfies it.
type ShipmentService interface {
	GetShipmentByOrderID(ctx context.Context, orderID string) (*domain.Shipment, error)
	CreateShipment(ctx context.Context, orderID string) (*domain.Shipment, error)
	CancelShipment(ctx context.Context, orderID string) error
}

// Server implements shippingv1.ShippingServiceServer.
type Server struct {
	shippingv1.UnimplementedShippingServiceServer

	svc ShipmentService
}

// NewServer creates a gRPC ShippingService server backed by the logic service.
func NewServer(svc ShipmentService) *Server {
	return &Server{svc: svc}
}

// GetShipmentByOrder mirrors GET /shipping/v1/internal/orders/{order_id}.
// A missing shipment is returned as an empty response (unset shipment), not an
// error, so callers can treat "no shipment yet" the same as the HTTP 404 path.
func (s *Server) GetShipmentByOrder(
	ctx context.Context,
	req *shippingv1.GetShipmentByOrderRequest,
) (*shippingv1.GetShipmentByOrderResponse, error) {
	shipment, err := s.svc.GetShipmentByOrderID(ctx, req.GetOrderId())
	if err != nil {
		if errors.Is(err, logicv1.ErrShipmentNotFound) {
			return &shippingv1.GetShipmentByOrderResponse{}, nil
		}
		return nil, status.Error(codes.Internal, "failed to get shipment")
	}
	if shipment == nil {
		return &shippingv1.GetShipmentByOrderResponse{}, nil
	}
	return &shippingv1.GetShipmentByOrderResponse{Shipment: toProto(shipment)}, nil
}

// CreateShipment creates a shipment for an order (order-fulfillment saga, step
// 2). Idempotent by order_id. The destination address is accepted but not
// persisted yet (forward-compat).
func (s *Server) CreateShipment(
	ctx context.Context,
	req *shippingv1.CreateShipmentRequest,
) (*shippingv1.CreateShipmentResponse, error) {
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	shipment, err := s.svc.CreateShipment(ctx, req.GetOrderId())
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to create shipment")
	}
	return &shippingv1.CreateShipmentResponse{Shipment: toProto(shipment)}, nil
}

// CancelShipment cancels the order's shipment (saga compensation). Idempotent.
func (s *Server) CancelShipment(
	ctx context.Context,
	req *shippingv1.CancelShipmentRequest,
) (*shippingv1.CancelShipmentResponse, error) {
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	if err := s.svc.CancelShipment(ctx, req.GetOrderId()); err != nil {
		return nil, status.Error(codes.Internal, "failed to cancel shipment")
	}
	return &shippingv1.CancelShipmentResponse{}, nil
}

// toProto maps the domain shipment to its protobuf representation. Timestamps
// are carried as the service's preformatted strings (empty when unset).
func toProto(s *domain.Shipment) *shippingv1.Shipment {
	estimatedDelivery := ""
	if s.EstimatedDelivery != nil {
		estimatedDelivery = *s.EstimatedDelivery
	}
	return &shippingv1.Shipment{
		Id:                strconv.Itoa(s.ID),
		OrderId:           strconv.Itoa(s.OrderID),
		TrackingNumber:    s.TrackingNumber,
		Carrier:           s.Carrier,
		Status:            s.Status,
		EstimatedDelivery: estimatedDelivery,
		CreatedAt:         s.CreatedAt,
		UpdatedAt:         s.UpdatedAt,
	}
}
