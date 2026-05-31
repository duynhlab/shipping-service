package postgres

import (
	"context"
	"errors"
	"fmt"
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
