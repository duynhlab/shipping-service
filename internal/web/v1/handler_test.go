package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/duynhlab/shipping-service/internal/core/domain"
	logicv1 "github.com/duynhlab/shipping-service/internal/logic/v1"
	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

// mockShipmentRepo is a configurable domain.ShipmentRepository double.
type mockShipmentRepo struct {
	shipment *domain.Shipment
	err      error
}

func (m *mockShipmentRepo) GetByTrackingNumber(_ context.Context, _ string) (*domain.Shipment, error) {
	return m.shipment, m.err
}
func (m *mockShipmentRepo) GetByOrderID(_ context.Context, _ string) (*domain.Shipment, error) {
	return m.shipment, m.err
}
func (m *mockShipmentRepo) CreateShipment(_ context.Context, _ string) (*domain.Shipment, error) {
	return m.shipment, m.err
}
func (m *mockShipmentRepo) CancelShipment(_ context.Context, _ string) error { return m.err }

func newHandler(repo domain.ShipmentRepository) *Handler {
	return NewHandler(logicv1.NewShippingService(repo))
}

func newCtx(method, target string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, target, nil)
	c.Params = params
	return c, rec
}

// decode returns the parsed JSON body.
func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body %q: %v", rec.Body.String(), err)
	}
	return body
}

func TestTrackShipment_Success(t *testing.T) {
	repo := &mockShipmentRepo{shipment: &domain.Shipment{ID: 1, TrackingNumber: "TN1", Status: "shipped"}}
	c, rec := newCtx(http.MethodGet, "/shipping/v1/public/track?tracking_number=TN1", nil)
	newHandler(repo).TrackShipment(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if decode(t, rec)["tracking_number"] != "TN1" {
		t.Errorf("tracking_number = %v, want TN1", decode(t, rec)["tracking_number"])
	}
}

func TestTrackShipment_LegacyParam(t *testing.T) {
	repo := &mockShipmentRepo{shipment: &domain.Shipment{ID: 2, TrackingNumber: "TN2", Status: "shipped"}}
	c, rec := newCtx(http.MethodGet, "/shipping/v1/public/track?trackingId=TN2", nil)
	newHandler(repo).TrackShipment(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestTrackShipment_MissingParam(t *testing.T) {
	c, rec := newCtx(http.MethodGet, "/shipping/v1/public/track", nil)
	newHandler(&mockShipmentRepo{}).TrackShipment(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "VALIDATION_ERROR" {
		t.Errorf("code = %v, want VALIDATION_ERROR", code)
	}
}

func TestTrackShipment_NotFound(t *testing.T) {
	repo := &mockShipmentRepo{err: domain.ErrShipmentNotFound}
	c, rec := newCtx(http.MethodGet, "/shipping/v1/public/track?tracking_number=missing", nil)
	newHandler(repo).TrackShipment(c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "NOT_FOUND" {
		t.Errorf("code = %v, want NOT_FOUND", code)
	}
}

func TestTrackShipment_InternalError(t *testing.T) {
	repo := &mockShipmentRepo{err: context.DeadlineExceeded}
	c, rec := newCtx(http.MethodGet, "/shipping/v1/public/track?tracking_number=boom", nil)
	newHandler(repo).TrackShipment(c)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "INTERNAL_ERROR" {
		t.Errorf("code = %v, want INTERNAL_ERROR", code)
	}
}

func TestEstimateShipping_Success(t *testing.T) {
	c, rec := newCtx(http.MethodGet, "/shipping/v1/public/estimate?origin=A&destination=B&weight=2.5", nil)
	newHandler(&mockShipmentRepo{}).EstimateShipping(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if decode(t, rec)["currency"] != "USD" {
		t.Errorf("currency = %v, want USD", decode(t, rec)["currency"])
	}
}

func TestEstimateShipping_MissingParams(t *testing.T) {
	c, rec := newCtx(http.MethodGet, "/shipping/v1/public/estimate?origin=A", nil)
	newHandler(&mockShipmentRepo{}).EstimateShipping(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "VALIDATION_ERROR" {
		t.Errorf("code = %v, want VALIDATION_ERROR", code)
	}
}

func TestEstimateShipping_InvalidWeight(t *testing.T) {
	cases := []string{"abc", "0", "-1", "NaN", "Inf"}
	for _, w := range cases {
		c, rec := newCtx(http.MethodGet, "/shipping/v1/public/estimate?origin=A&destination=B&weight="+w, nil)
		newHandler(&mockShipmentRepo{}).EstimateShipping(c)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("weight=%q: status = %d, want 400", w, rec.Code)
		}
		if code := decode(t, rec)["code"]; code != "VALIDATION_ERROR" {
			t.Errorf("weight=%q: code = %v, want VALIDATION_ERROR", w, code)
		}
	}
}

func TestGetShipmentByOrder_Success(t *testing.T) {
	repo := &mockShipmentRepo{shipment: &domain.Shipment{ID: 5, OrderID: 42, Status: "delivered"}}
	c, rec := newCtx(http.MethodGet, "/shipping/v1/internal/orders/42", gin.Params{{Key: "orderId", Value: "42"}})
	newHandler(repo).GetShipmentByOrder(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if decode(t, rec)["id"].(float64) != 5 {
		t.Errorf("id = %v, want 5", decode(t, rec)["id"])
	}
}

func TestGetShipmentByOrder_NotFound(t *testing.T) {
	repo := &mockShipmentRepo{err: domain.ErrShipmentNotFound}
	c, rec := newCtx(http.MethodGet, "/shipping/v1/internal/orders/9", gin.Params{{Key: "orderId", Value: "9"}})
	newHandler(repo).GetShipmentByOrder(c)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "NOT_FOUND" {
		t.Errorf("code = %v, want NOT_FOUND", code)
	}
}

func TestGetShipmentByOrder_InternalError(t *testing.T) {
	repo := &mockShipmentRepo{err: context.DeadlineExceeded}
	c, rec := newCtx(http.MethodGet, "/shipping/v1/internal/orders/9", gin.Params{{Key: "orderId", Value: "9"}})
	newHandler(repo).GetShipmentByOrder(c)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "INTERNAL_ERROR" {
		t.Errorf("code = %v, want INTERNAL_ERROR", code)
	}
}
