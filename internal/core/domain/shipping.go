package domain

type Shipment struct {
	ID                int     `json:"id"`
	OrderID           int     `json:"order_id"`
	TrackingNumber    string  `json:"tracking_number"`
	Carrier           string  `json:"carrier,omitempty"`
	Status            string  `json:"status"`
	EstimatedDelivery *string `json:"estimated_delivery,omitempty"`
	CreatedAt         string  `json:"created_at,omitempty"`
	UpdatedAt         string  `json:"updated_at,omitempty"`
}

type EstimateRequest struct {
	Origin      string  `json:"origin" binding:"required"`
	Destination string  `json:"destination" binding:"required"`
	Weight      float64 `json:"weight" binding:"required"`
}

// Quote is the GetQuote answer (RFC-0015 P3): the shipping fee for a
// method × region pair, in int64 minor units, plus an indicative ETA.
type Quote struct {
	FeeMinor int64
	ETADays  int32
}

type EstimateResponse struct {
	Origin           string  `json:"origin"`
	Destination      string  `json:"destination"`
	Weight           float64 `json:"weight"`
	EstimatedCost    float64 `json:"estimated_cost"`
	EstimatedDays    int     `json:"estimated_days"`
	Currency         string  `json:"currency"`
	Carrier          string  `json:"carrier"`
}
