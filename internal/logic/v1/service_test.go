package v1

import (
	"context"
	"errors"
	"testing"

	"github.com/duynhlab/shipping-service/internal/core/domain"
)

// mockShipmentRepository is a configurable test double for
// domain.ShipmentRepository.
type mockShipmentRepository struct {
	shipment *domain.Shipment
	err      error
}

func (m *mockShipmentRepository) GetByTrackingNumber(ctx context.Context, trackingNumber string) (*domain.Shipment, error) {
	return m.shipment, m.err
}

func (m *mockShipmentRepository) GetByOrderID(ctx context.Context, orderID string) (*domain.Shipment, error) {
	return m.shipment, m.err
}

func TestEstimateShipping(t *testing.T) {
	service := NewShippingService(nil)
	ctx := context.Background()

	tests := []struct {
		name        string
		origin      string
		destination string
		weight      float64
		wantCost    float64
		wantDays    int
		wantErr     bool
	}{
		{
			name:        "Same city, light package",
			origin:      "NY",
			destination: "NY",
			weight:      2.0,
			// Cost = 5.0 (base) + 2.0*1.5 (weight) + 0 (distance) = 8.0
			wantCost: 8.0,
			wantDays: 3,
			wantErr:  false,
		},
		{
			name:        "Different city, light package",
			origin:      "NY",
			destination: "CA",
			weight:      2.0,
			// Cost = 5.0 (base) + 2.0*1.5 (weight) + 10.0 (distance) = 18.0
			wantCost: 18.0,
			wantDays: 5,
			wantErr:  false,
		},
		{
			name:        "Same city, heavy package",
			origin:      "NY",
			destination: "NY",
			weight:      12.0,
			// Cost = 5.0 (base) + 12.0*1.5 (weight) + 0 (distance) = 23.0
			// Days = 3 + 2 (heavy penalty) = 5
			wantCost: 23.0,
			wantDays: 5,
			wantErr:  false,
		},
		{
			name:        "Different city, heavy package",
			origin:      "NY",
			destination: "CA",
			weight:      12.0,
			// Cost = 5.0 (base) + 12.0*1.5 (weight) + 10.0 (distance) = 33.0
			// Days = 5 + 2 (heavy penalty) = 7
			wantCost: 33.0,
			wantDays: 7,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.EstimateShipping(ctx, tt.origin, tt.destination, tt.weight)
			if (err != nil) != tt.wantErr {
				t.Errorf("EstimateShipping() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.EstimatedCost != tt.wantCost {
				t.Errorf("EstimateShipping() cost = %v, want %v", got.EstimatedCost, tt.wantCost)
			}
			if got.EstimatedDays != tt.wantDays {
				t.Errorf("EstimateShipping() days = %v, want %v", got.EstimatedDays, tt.wantDays)
			}
		})
	}
}

func TestTrackShipment(t *testing.T) {
	ctx := context.Background()
	repoErr := errors.New("db connection lost")

	tests := []struct {
		name           string
		trackingNumber string
		repo           *mockShipmentRepository
		want           *domain.Shipment
		wantErr        error
	}{
		{
			name:           "found",
			trackingNumber: "TRK123",
			repo: &mockShipmentRepository{
				shipment: &domain.Shipment{ID: 1, OrderID: 10, TrackingNumber: "TRK123", Carrier: "DHL", Status: "in_transit"},
			},
			want: &domain.Shipment{ID: 1, OrderID: 10, TrackingNumber: "TRK123", Carrier: "DHL", Status: "in_transit"},
		},
		{
			name:           "not found maps to logic sentinel",
			trackingNumber: "MISSING",
			repo:           &mockShipmentRepository{err: domain.ErrShipmentNotFound},
			wantErr:        ErrShipmentNotFound,
		},
		{
			name:           "repo error propagates",
			trackingNumber: "TRK999",
			repo:           &mockShipmentRepository{err: repoErr},
			wantErr:        repoErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewShippingService(tt.repo)

			got, err := service.TrackShipment(ctx, tt.trackingNumber)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("TrackShipment() error = %v, want %v", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("TrackShipment() shipment = %v, want nil on error", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("TrackShipment() unexpected error = %v", err)
			}
			if got == nil || got.ID != tt.want.ID || got.TrackingNumber != tt.want.TrackingNumber || got.Status != tt.want.Status {
				t.Errorf("TrackShipment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetShipmentByOrderID(t *testing.T) {
	ctx := context.Background()
	repoErr := errors.New("query timeout")

	tests := []struct {
		name    string
		orderID string
		repo    *mockShipmentRepository
		want    *domain.Shipment
		wantErr error
	}{
		{
			name:    "found",
			orderID: "10",
			repo: &mockShipmentRepository{
				shipment: &domain.Shipment{ID: 2, OrderID: 10, TrackingNumber: "TRK456", Status: "delivered"},
			},
			want: &domain.Shipment{ID: 2, OrderID: 10, TrackingNumber: "TRK456", Status: "delivered"},
		},
		{
			name:    "not found maps to logic sentinel",
			orderID: "404",
			repo:    &mockShipmentRepository{err: domain.ErrShipmentNotFound},
			wantErr: ErrShipmentNotFound,
		},
		{
			name:    "repo error propagates",
			orderID: "500",
			repo:    &mockShipmentRepository{err: repoErr},
			wantErr: repoErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewShippingService(tt.repo)

			got, err := service.GetShipmentByOrderID(ctx, tt.orderID)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("GetShipmentByOrderID() error = %v, want %v", err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("GetShipmentByOrderID() shipment = %v, want nil on error", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("GetShipmentByOrderID() unexpected error = %v", err)
			}
			if got == nil || got.ID != tt.want.ID || got.OrderID != tt.want.OrderID || got.Status != tt.want.Status {
				t.Errorf("GetShipmentByOrderID() = %v, want %v", got, tt.want)
			}
		})
	}
}
