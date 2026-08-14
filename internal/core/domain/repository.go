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

	// ListShipments returns one operator page (newest first) plus the unpaged
	// total; status narrows to one as-built value when set (RFC-0023 slice A —
	// the Backoffice's cross-customer view; there is no owner scope on
	// shipments to begin with).
	ListShipments(ctx context.Context, status string, limit, offset int) ([]Shipment, int, error)

	// GetByID returns one shipment by primary key for the operator case view.
	GetByID(ctx context.Context, id int) (*Shipment, error)
}
