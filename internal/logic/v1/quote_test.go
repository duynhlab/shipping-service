package v1

import (
	"context"
	"errors"
	"testing"
)

func TestGetQuote_RateTable(t *testing.T) {
	svc := &ShippingService{}
	cases := map[string]struct {
		method, region string
		fee            int64
		eta            int32
	}{
		"standard domestic":      {"standard", "VN", 300, 5},
		"standard international": {"standard", "US", 900, 10},
		"express domestic":       {"express", "VN", 700, 2},
		"express international":  {"express", "DE", 1900, 5},
		"case-insensitive":       {"Express", "vn", 700, 2},
	}
	for name, tc := range cases {
		q, err := svc.GetQuote(context.Background(), tc.method, tc.region)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if q.FeeMinor != tc.fee || q.ETADays != tc.eta {
			t.Errorf("%s: quote = %+v, want fee %d eta %d", name, q, tc.fee, tc.eta)
		}
	}
}

func TestGetQuote_UnknownInputRejected(t *testing.T) {
	svc := &ShippingService{}
	for name, in := range map[string][2]string{
		"unknown method": {"drone", "VN"},
		"empty method":   {"", "VN"},
		"empty region":   {"standard", ""},
	} {
		if _, err := svc.GetQuote(context.Background(), in[0], in[1]); !errors.Is(err, ErrUnknownQuoteInput) {
			t.Errorf("%s: err = %v, want ErrUnknownQuoteInput", name, err)
		}
	}
}
