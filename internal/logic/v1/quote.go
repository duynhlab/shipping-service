package v1

import (
	"context"
	"errors"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/duynhlab/pkg/obsx"
	"github.com/duynhlab/shipping-service/internal/core/domain"
)

// ErrUnknownQuoteInput — the method or region is not priceable; the gRPC
// layer maps it to INVALID_ARGUMENT so checkout can answer 400.
var ErrUnknownQuoteInput = errors.New("unknown shipping method or region")

// homeRegion is the domestic bucket of the rate table.
const homeRegion = "VN"

// quoteRates is the static method × region rate table (RFC-0015 P3):
// shipping-service is the quote authority — these numbers replace the
// hardcoded $5 fee the platform carried since the first cart demo. Money is
// int64 minor units. Two region buckets for now: domestic (VN) and rest of
// world; real carrier tables can replace this without changing the contract.
var quoteRates = map[string]struct {
	domesticFee, intlFee int64
	domesticETA, intlETA int32
}{
	"standard": {domesticFee: 300, intlFee: 900, domesticETA: 5, intlETA: 10},
	"express":  {domesticFee: 700, intlFee: 1900, domesticETA: 2, intlETA: 5},
}

// GetQuote prices a shipping method for a destination region. Pure read.
func (s *ShippingService) GetQuote(ctx context.Context, method, region string) (*domain.Quote, error) {
	_, span := obsx.StartSpan(ctx, tracerScope, "shipping.quote", trace.WithAttributes(
		attribute.String("layer", "logic"),
		attribute.String("quote.method", method),
		attribute.String("quote.region", region),
	))
	defer span.End()

	rate, ok := quoteRates[strings.ToLower(method)]
	if !ok || region == "" {
		return nil, ErrUnknownQuoteInput
	}
	q := &domain.Quote{FeeMinor: rate.intlFee, ETADays: rate.intlETA}
	if strings.EqualFold(region, homeRegion) {
		q.FeeMinor, q.ETADays = rate.domesticFee, rate.domesticETA
	}
	span.SetAttributes(
		attribute.Int64("quote.fee_minor", q.FeeMinor),
		attribute.Int("quote.eta_days", int(q.ETADays)),
	)
	return q, nil
}
