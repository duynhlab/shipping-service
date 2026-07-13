package v1

import (
	"context"
	"testing"

	shippingv1 "github.com/duynhlab/pkg/proto/shipping/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	logicv1 "github.com/duynhlab/shipping-service/internal/logic/v1"
)

func TestGetQuote_MapsFeeAndETA(t *testing.T) {
	srv := NewServer(&logicv1.ShippingService{})

	resp, err := srv.GetQuote(context.Background(), &shippingv1.GetQuoteRequest{Method: "standard", Region: "VN"})
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if resp.GetFeeMinor() != 300 || resp.GetEtaDays() != 5 {
		t.Errorf("quote = %+v, want 300 minor / 5 days (domestic standard)", resp)
	}
}

func TestGetQuote_UnknownInputIsInvalidArgument(t *testing.T) {
	srv := NewServer(&logicv1.ShippingService{})

	_, err := srv.GetQuote(context.Background(), &shippingv1.GetQuoteRequest{Method: "drone", Region: "VN"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}
