package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/duynhlab/pkg/authmw"
	"github.com/duynhlab/shipping-service/internal/core/domain"
	logicv1 "github.com/duynhlab/shipping-service/internal/logic/v1"
)

// listRepo extends the shared mock with the protected read surface.
type listRepo struct {
	mockShipmentRepo
	items []domain.Shipment
	total int
	err   error
	got   struct {
		status        string
		limit, offset int
	}
	byID map[int]*domain.Shipment
}

func (m *listRepo) ListShipments(_ context.Context, status string, limit, offset int) ([]domain.Shipment, int, error) {
	m.got.status, m.got.limit, m.got.offset = status, limit, offset
	return m.items, m.total, m.err
}

func (m *listRepo) GetByID(_ context.Context, id int) (*domain.Shipment, error) {
	if s, ok := m.byID[id]; ok {
		return s, nil
	}
	return nil, domain.ErrShipmentNotFound
}

func operatorEngine(t *testing.T, repo domain.ShipmentRepository, roles ...string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler(logicv1.NewShippingService(repo))
	h.mountProtected(r,
		func(c *gin.Context) {
			c.Set(authmw.CtxUserID, "d0e00000-0000-4000-8000-000000000001")
			c.Set(authmw.CtxRoles, roles)
			c.Next()
		},
		authmw.MiddlewareRequireRole(backofficeRole))
	return r
}

func TestProtectedRoleGate(t *testing.T) {
	r := operatorEngine(t, &listRepo{}, "customer")
	for _, path := range []string{"/shipping/v1/protected/shipments", "/shipping/v1/protected/shipments/1"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: want 403, got %d", path, w.Code)
		}
	}
}

func TestListShipments(t *testing.T) {
	repo := &listRepo{
		items: []domain.Shipment{{ID: 9, OrderID: 4, TrackingNumber: "TRK-9", Status: "pending"}},
		total: 23,
	}
	r := operatorEngine(t, repo, backofficeRole)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shipping/v1/protected/shipments?page=2&page_size=10&status=pending", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if repo.got.status != "pending" || repo.got.limit != 10 || repo.got.offset != 10 {
		t.Fatalf("filter/paging not forwarded: %+v", repo.got)
	}
	var resp struct {
		TotalItems int `json:"total_items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.TotalItems != 23 {
		t.Fatalf("want total 23, got %d", resp.TotalItems)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shipping/v1/protected/shipments?status=bogus", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bogus status: want 400, got %d", w.Code)
	}
}

func TestGetShipment(t *testing.T) {
	repo := &listRepo{byID: map[int]*domain.Shipment{7: {ID: 7, Status: "pending"}}}
	r := operatorEngine(t, repo, backofficeRole)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shipping/v1/protected/shipments/7", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shipping/v1/protected/shipments/999", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", w.Code)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shipping/v1/protected/shipments/abc", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", w.Code)
	}
}

func TestRegisterProtectedRoutesRealChain(t *testing.T) {
	verifier, err := authmw.NewVerifier(authmw.Config{
		Issuer:   "http://localhost:8081/realms/duynhlab-staff",
		Audience: "duynhlab-platform",
	})
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterProtectedRoutes(r, NewHandler(logicv1.NewShippingService(&listRepo{})), verifier)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shipping/v1/protected/shipments", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("tokenless: want 401 from the real chain, got %d", w.Code)
	}
}

func TestListShipmentsRepoError(t *testing.T) {
	r := operatorEngine(t, &listRepo{err: context.DeadlineExceeded}, backofficeRole)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/shipping/v1/protected/shipments", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", w.Code)
	}
}
