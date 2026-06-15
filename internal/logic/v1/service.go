package v1

import (
	"context"
	"errors"

	"github.com/duynhlab/shipping-service/internal/core/domain"
	"github.com/duynhlab/shipping-service/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ShippingService struct {
	repo domain.ShipmentRepository
}

func NewShippingService(repo domain.ShipmentRepository) *ShippingService {
	return &ShippingService{
		repo: repo,
	}
}

func (s *ShippingService) TrackShipment(ctx context.Context, trackingNumber string) (*domain.Shipment, error) {
	ctx, span := middleware.StartSpan(ctx, "shipping.track", trace.WithAttributes(
		attribute.String("layer", "logic"),
		attribute.String("api.version", "v1"),
		attribute.String("tracking.number", trackingNumber),
	))
	defer span.End()

	shipment, err := s.repo.GetByTrackingNumber(ctx, trackingNumber)
	if err != nil {
		if errors.Is(err, domain.ErrShipmentNotFound) {
			span.SetAttributes(attribute.Bool("shipment.found", false))
			return nil, ErrShipmentNotFound
		}
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.Bool("shipment.found", true),
		attribute.Int("shipment.id", shipment.ID),
		attribute.String("shipment.status", shipment.Status),
		attribute.String("shipment.carrier", shipment.Carrier),
	)

	return shipment, nil
}

// EstimateShipping calculates estimated shipping cost and delivery time
func (s *ShippingService) EstimateShipping(ctx context.Context, origin, destination string, weight float64) (*domain.EstimateResponse, error) {
	_, span := middleware.StartSpan(ctx, "shipping.estimate", trace.WithAttributes(
		attribute.String("layer", "logic"),
		attribute.String("api.version", "v1"),
		attribute.String("origin", origin),
		attribute.String("destination", destination),
		attribute.Float64("weight", weight),
	))
	defer span.End()

	// Simple deterministic calculation for demo purposes
	// In production, this would call external carrier APIs
	baseCost := 5.0
	weightCost := weight * 1.5
	distanceCost := 0.0
	estimatedDays := 3

	// Simple distance estimation based on string comparison
	if origin != destination {
		distanceCost = 10.0
		estimatedDays = 5
	}

	// Heavier packages take longer
	if weight > 10 {
		estimatedDays += 2
	}

	totalCost := baseCost + weightCost + distanceCost

	response := &domain.EstimateResponse{
		Origin:        origin,
		Destination:   destination,
		Weight:        weight,
		EstimatedCost: totalCost,
		EstimatedDays: estimatedDays,
		Currency:      "USD",
		Carrier:       "Standard Shipping",
	}

	span.SetAttributes(
		attribute.Float64("estimate.cost", totalCost),
		attribute.Int("estimate.days", estimatedDays),
	)

	return response, nil
}

// GetShipmentByOrderID retrieves a shipment by its order ID
func (s *ShippingService) GetShipmentByOrderID(ctx context.Context, orderID string) (*domain.Shipment, error) {
	ctx, span := middleware.StartSpan(ctx, "shipping.get_by_order", trace.WithAttributes(
		attribute.String("layer", "logic"),
		attribute.String("api.version", "v1"),
		attribute.String("order_id", orderID),
	))
	defer span.End()

	shipment, err := s.repo.GetByOrderID(ctx, orderID)
	if err != nil {
		if errors.Is(err, domain.ErrShipmentNotFound) {
			span.SetAttributes(attribute.Bool("shipment.found", false))
			return nil, ErrShipmentNotFound
		}
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.Bool("shipment.found", true),
		attribute.Int("shipment.id", shipment.ID),
		attribute.String("shipment.status", shipment.Status),
	)

	return shipment, nil
}

// CreateShipment creates a shipment for an order (order-fulfillment saga, step
// 2). Idempotent by orderID: a repeat call returns the existing shipment. The
// destination address from the saga is not persisted yet (the shipment is keyed
// by order); add a column when carrier integration needs it.
func (s *ShippingService) CreateShipment(ctx context.Context, orderID string) (*domain.Shipment, error) {
	ctx, span := middleware.StartSpan(ctx, "shipping.create", trace.WithAttributes(
		attribute.String("layer", "logic"),
		attribute.String("api.version", "v1"),
		attribute.String("order_id", orderID),
	))
	defer span.End()

	shipment, err := s.repo.CreateShipment(ctx, orderID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	span.SetAttributes(
		attribute.Int("shipment.id", shipment.ID),
		attribute.String("shipment.tracking_number", shipment.TrackingNumber),
	)
	return shipment, nil
}

// CancelShipment cancels the order's shipment (saga compensation for
// CreateShipment). Idempotent by orderID.
func (s *ShippingService) CancelShipment(ctx context.Context, orderID string) error {
	ctx, span := middleware.StartSpan(ctx, "shipping.cancel", trace.WithAttributes(
		attribute.String("layer", "logic"),
		attribute.String("api.version", "v1"),
		attribute.String("order_id", orderID),
	))
	defer span.End()

	if err := s.repo.CancelShipment(ctx, orderID); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}
