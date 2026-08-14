package v1

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/duynhlab/pkg/authmw"
	"github.com/duynhlab/pkg/httpx"
	logicv1 "github.com/duynhlab/shipping-service/internal/logic/v1"
)

// Protected Backoffice reads (RFC-0023 slice A, ADR-047/050): shipping's
// first authenticated HTTP surface. The staff-realm verifier is authoritative
// (the edge's jwt-edge-staff check is coarse), and every route additionally
// requires the backoffice_admin role. Shipments carry no owner scope, so
// these are plain cross-customer reads.
//
// Status stays the AS-BUILT vocabulary: code writes only `pending` and
// `cancelled`; `in_transit`/`delivered` exist in dev seeds. There is no FSM —
// transitions are deliberately NOT exposed (RFC-0023 non-goal).

// backofficeRole is the staff-realm role every protected route requires.
const backofficeRole = "backoffice_admin"

// validShipmentStatuses mirrors the values that exist as-built (two written
// by code, two seed-only). Anything else is a typo, not a filter.
var validShipmentStatuses = map[string]bool{
	"pending": true, "cancelled": true, "in_transit": true, "delivered": true,
}

// RegisterProtectedRoutes mounts the Backoffice group with the real guard
// chain. Split from mountProtected so tests can inject fakes.
func RegisterProtectedRoutes(r *gin.Engine, h *Handler, staffVerifier *authmw.Verifier) {
	h.mountProtected(r, authmw.MiddlewareJWT(staffVerifier), authmw.MiddlewareRequireRole(backofficeRole))
}

func (h *Handler) mountProtected(r *gin.Engine, authMW ...gin.HandlerFunc) {
	protected := r.Group("/shipping/v1/protected")
	protected.Use(authMW...)
	{
		protected.GET("/shipments", h.ListShipments)
		protected.GET("/shipments/:id", h.GetShipment)
	}
}

// ListShipments serves GET /shipments?status=&page=&page_size=.
func (h *Handler) ListShipments(c *gin.Context) {
	page, pageSize := httpx.ParsePage(c)

	status := c.Query("status")
	if status != "" && !validShipmentStatuses[status] {
		httpx.RespondError(c, http.StatusBadRequest, httpx.CodeValidation,
			"status must be one of pending, cancelled, in_transit, delivered")
		return
	}

	items, total, err := h.service.ListShipments(c.Request.Context(), status, pageSize, httpx.Offset(page, pageSize))
	if err != nil {
		httpx.RespondError(c, http.StatusInternalServerError, httpx.CodeInternal, "Internal server error")
		return
	}
	c.JSON(http.StatusOK, httpx.NewPaginated(items, page, pageSize, total))
}

// GetShipment serves GET /shipments/:id — the operator case view.
func (h *Handler) GetShipment(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		httpx.RespondError(c, http.StatusBadRequest, httpx.CodeValidation,
			"id must be a positive integer")
		return
	}

	shipment, err := h.service.GetShipment(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, logicv1.ErrShipmentNotFound) {
			httpx.RespondError(c, http.StatusNotFound, httpx.CodeNotFound, "Shipment not found")
			return
		}
		httpx.RespondError(c, http.StatusInternalServerError, httpx.CodeInternal, "Internal server error")
		return
	}
	c.JSON(http.StatusOK, shipment)
}
