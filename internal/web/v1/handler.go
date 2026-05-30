package v1

import (
	"errors"
	"math"
	"net/http"
	"strconv"

	logicv1 "github.com/duynhne/shipping-service/internal/logic/v1"
	"github.com/duynhne/shipping-service/middleware"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type Handler struct {
	service *logicv1.ShippingService
}

func NewHandler(service *logicv1.ShippingService) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) TrackShipment(c *gin.Context) {
	ctx, span := middleware.StartSpan(c.Request.Context(), "http.request", trace.WithAttributes(
		attribute.String("layer", "web"),
		attribute.String("method", c.Request.Method),
		attribute.String("path", c.Request.URL.Path),
	))
	defer span.End()

	zapLogger := middleware.GetLoggerFromGinContext(c)

	// Accept both tracking_number (preferred, per API docs) and trackingId (legacy)
	trackingID := c.Query("tracking_number")
	if trackingID == "" {
		trackingID = c.Query("trackingId") // Backward compatibility
	}
	span.SetAttributes(attribute.String("tracking.id", trackingID))

	if trackingID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing required parameter: tracking_number"})
		return
	}

	shipment, err := h.service.TrackShipment(ctx, trackingID)
	if err != nil {
		span.RecordError(err)
		zapLogger.Error("Failed to track shipment", zap.Error(err))

		switch {
		case errors.Is(err, logicv1.ErrShipmentNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Shipment not found"})
		case errors.Is(err, logicv1.ErrCarrierUnavailable):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Carrier unavailable"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
		return
	}

	zapLogger.Info("Shipment tracked", zap.String("tracking_id", trackingID))
	c.JSON(http.StatusOK, shipment)
}

// EstimateShipping handles GET /shipping/v1/public/estimate
// Query params: origin, destination, weight
func (h *Handler) EstimateShipping(c *gin.Context) {
	ctx, span := middleware.StartSpan(c.Request.Context(), "http.request", trace.WithAttributes(
		attribute.String("layer", "web"),
		attribute.String("method", c.Request.Method),
		attribute.String("path", c.Request.URL.Path),
	))
	defer span.End()

	zapLogger := middleware.GetLoggerFromGinContext(c)

	origin := c.Query("origin")
	destination := c.Query("destination")
	weightStr := c.Query("weight")

	// Validate required params
	if origin == "" || destination == "" || weightStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Missing required parameters: origin, destination, weight",
		})
		return
	}

	// Parse weight
	weight, err := strconv.ParseFloat(weightStr, 64)
	if err != nil || weight <= 0 || math.IsNaN(weight) || math.IsInf(weight, 0) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "weight must be a positive number"})
		return
	}

	span.SetAttributes(
		attribute.String("estimate.origin", origin),
		attribute.String("estimate.destination", destination),
		attribute.Float64("estimate.weight", weight),
	)

	estimate, err := h.service.EstimateShipping(ctx, origin, destination, weight)
	if err != nil {
		span.RecordError(err)
		zapLogger.Error("Failed to estimate shipping", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	zapLogger.Info("Shipping estimated",
		zap.String("origin", origin),
		zap.String("destination", destination),
		zap.Float64("weight", weight),
		zap.Float64("cost", estimate.EstimatedCost),
	)
	c.JSON(http.StatusOK, estimate)
}

// GetShipmentByOrder handles GET /shipping/v1/internal/orders/:orderId
// Returns shipment info for a given order ID
func (h *Handler) GetShipmentByOrder(c *gin.Context) {
	ctx, span := middleware.StartSpan(c.Request.Context(), "http.request", trace.WithAttributes(
		attribute.String("layer", "web"),
		attribute.String("method", c.Request.Method),
		attribute.String("path", c.Request.URL.Path),
	))
	defer span.End()

	zapLogger := middleware.GetLoggerFromGinContext(c)

	orderID := c.Param("orderId")
	span.SetAttributes(attribute.String("order.id", orderID))

	shipment, err := h.service.GetShipmentByOrderID(ctx, orderID)
	if err != nil {
		span.RecordError(err)
		zapLogger.Error("Failed to get shipment by order", zap.Error(err), zap.String("order_id", orderID))

		switch {
		case errors.Is(err, logicv1.ErrShipmentNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Shipment not found for this order"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		}
		return
	}

	zapLogger.Info("Shipment retrieved by order", zap.String("order_id", orderID), zap.Int("shipment_id", shipment.ID))
	c.JSON(http.StatusOK, shipment)
}
