package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/duynhlab/shipping-service/internal/core/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ShipmentRepository struct {
	db *pgxpool.Pool
}

func NewShipmentRepository(db *pgxpool.Pool) *ShipmentRepository {
	return &ShipmentRepository{db: db}
}

func (r *ShipmentRepository) GetByTrackingNumber(ctx context.Context, trackingNumber string) (*domain.Shipment, error) {
	query := `
		SELECT id, order_id, tracking_number, carrier, status, estimated_delivery, created_at, updated_at
		FROM shipments
		WHERE tracking_number = $1
		LIMIT 1
	`

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	row := r.db.QueryRow(ctx, query, trackingNumber)
	shipment, err := r.scanShipment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("track shipment with number %q: %w", trackingNumber, domain.ErrShipmentNotFound)
		}
		return nil, fmt.Errorf("query shipment: %w", err)
	}

	return shipment, nil
}

func (r *ShipmentRepository) GetByOrderID(ctx context.Context, orderID string) (*domain.Shipment, error) {
	query := `
		SELECT id, order_id, tracking_number, carrier, status, estimated_delivery, created_at, updated_at
		FROM shipments
		WHERE order_id = $1
		LIMIT 1
	`

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	row := r.db.QueryRow(ctx, query, orderID)
	shipment, err := r.scanShipment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get shipment for order %q: %w", orderID, domain.ErrShipmentNotFound)
		}
		return nil, fmt.Errorf("query shipment: %w", err)
	}

	return shipment, nil
}

// CreateShipment creates a shipment for an order, or returns the existing one if
// the order already has one (idempotent by order_id via the unique constraint).
// The tracking number is derived from the order id, so a retry is idempotent on
// the tracking_number unique constraint too.
func (r *ShipmentRepository) CreateShipment(ctx context.Context, orderID string) (*domain.Shipment, error) {
	oid, err := strconv.Atoi(orderID)
	if err != nil {
		return nil, fmt.Errorf("create shipment: invalid order id %q: %w", orderID, err)
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	tracking := fmt.Sprintf("MOP%010d", oid)
	const carrier = "MOP Express"

	// ON CONFLICT ... DO UPDATE (a no-op touch of order_id) always RETURNs the
	// row, so a concurrent duplicate CreateShipment gets the existing shipment
	// atomically — no read-after-conflict race with a separate SELECT.
	query := `
		INSERT INTO shipments (order_id, tracking_number, carrier, status, estimated_delivery)
		VALUES ($1, $2, $3, 'pending', NOW() + INTERVAL '5 days')
		ON CONFLICT (order_id) DO UPDATE SET order_id = EXCLUDED.order_id
		RETURNING id, order_id, tracking_number, carrier, status, estimated_delivery, created_at, updated_at
	`
	row := r.db.QueryRow(ctx, query, oid, tracking, carrier)
	shipment, err := r.scanShipment(row)
	if err != nil {
		return nil, fmt.Errorf("create shipment for order %q: %w", orderID, err)
	}
	return shipment, nil
}

// CancelShipment marks the order's shipment cancelled. Idempotent: zero rows
// affected (no shipment, or already cancelled) is still a success.
func (r *ShipmentRepository) CancelShipment(ctx context.Context, orderID string) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	_, err := r.db.Exec(ctx,
		`UPDATE shipments SET status = 'cancelled', updated_at = CURRENT_TIMESTAMP
		 WHERE order_id = $1 AND status <> 'cancelled'`,
		orderID)
	if err != nil {
		return fmt.Errorf("cancel shipment for order %q: %w", orderID, err)
	}
	return nil
}

func (r *ShipmentRepository) scanShipment(row pgx.Row) (*domain.Shipment, error) {
	var id, orderID int
	var trackingNum, status string
	var carrier *string
	var estimatedDelivery *time.Time
	var createdAt, updatedAt time.Time

	err := row.Scan(
		&id, &orderID, &trackingNum, &carrier, &status, &estimatedDelivery, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	shipment := &domain.Shipment{
		ID:             id,
		OrderID:        orderID,
		TrackingNumber: trackingNum,
		Status:         status,
		CreatedAt:      createdAt.Format(time.RFC3339),
		UpdatedAt:      updatedAt.Format(time.RFC3339),
	}

	if carrier == nil {
		shipment.Carrier = ""
	} else {
		shipment.Carrier = *carrier
	}

	if estimatedDelivery != nil {
		deliveryStr := estimatedDelivery.Format(time.RFC3339)
		shipment.EstimatedDelivery = &deliveryStr
	}

	return shipment, nil
}
