package domain

import "context"

// ShipmentRepository defines the interface for shipment data access.
type ShipmentRepository interface {
	GetByTrackingNumber(ctx context.Context, trackingNumber string) (*Shipment, error)
	GetByOrderID(ctx context.Context, orderID string) (*Shipment, error)

	// CreateShipment creates a shipment for an order, or returns the existing one
	// (idempotent by orderID) — the order-fulfillment saga's CreateShipment step.
	CreateShipment(ctx context.Context, orderID string) (*Shipment, error)

	// CancelShipment marks the order's shipment cancelled (the saga compensation
	// for CreateShipment). Idempotent: a no-op when there is no shipment or it is
	// already cancelled.
	CancelShipment(ctx context.Context, orderID string) error
}
