package main

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/duynhlab/pkg/authmw"
	"github.com/duynhlab/pkg/logger/slogx"
	"github.com/duynhlab/shipping-service/config"
)

// The ADR-017 contract step: the pre-v3 verb paths are gone and only the
// collection-noun forms are mounted.
func TestPublicRoutesAreCanonicalOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verifier, err := authmw.NewVerifier(authmw.Config{
		Issuer:   "http://localhost:8081/realms/duynhlab-staff",
		Audience: "duynhlab-platform",
	})
	if err != nil {
		t.Fatalf("verifier: %v", err)
	}
	srv := setupServer(&config.Config{}, "shipping", slogx.New(slogx.Config{}), &atomic.Bool{}, nil, nil, verifier)
	engine, ok := srv.Handler.(*gin.Engine)
	if !ok {
		t.Fatalf("handler is %T, want *gin.Engine", srv.Handler)
	}
	mounted := map[string]bool{}
	for _, r := range engine.Routes() {
		if r.Method == http.MethodGet {
			mounted[r.Path] = true
		}
	}
	for _, p := range []string{"/shipping/v1/public/shipments/track", "/shipping/v1/public/shipments/estimate"} {
		if !mounted[p] {
			t.Errorf("canonical route %s is not mounted", p)
		}
	}
	for _, p := range []string{"/shipping/v1/public/track", "/shipping/v1/public/estimate"} {
		if mounted[p] {
			t.Errorf("ADR-017 alias %s is still mounted", p)
		}
	}
}
