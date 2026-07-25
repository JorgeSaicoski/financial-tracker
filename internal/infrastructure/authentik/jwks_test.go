package authentik

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseECKeyRejectsPointNotOnCurve(t *testing.T) {
	// A garbage (x, y) pair — astronomically unlikely to land on P-256.
	notOnCurve := jwk{
		Kty: "EC",
		Kid: "bad-ec-key",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString([]byte{1, 2, 3, 4}),
		Y:   base64.RawURLEncoding.EncodeToString([]byte{5, 6, 7, 8}),
	}
	if _, err := parseECKey(notOnCurve); err == nil {
		t.Fatal("want error for EC point not on the declared curve, got nil")
	}
}

func TestGetBodyRejectsOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", maxDiscoveryBodyBytes+1)))
	}))
	t.Cleanup(srv.Close)

	if _, err := getBody(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Fatal("want error for oversized response body, got nil")
	}
}

func TestGetBodyAcceptsResponseAtLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", maxDiscoveryBodyBytes)))
	}))
	t.Cleanup(srv.Close)

	body, err := getBody(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(body) != maxDiscoveryBodyBytes {
		t.Errorf("body length = %d, want %d", len(body), maxDiscoveryBodyBytes)
	}
}
