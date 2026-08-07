package simpleauth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

// testDiscovery/testJWK/testJWKSet mirror the JSON shape
// infrastructure/authentik's (private) oidcDiscovery/jwk/jwkSet types
// use — that package's own tests already cover the discovery/JWKS
// parsing mechanics this package reuses via authentik.Client, so these
// exist here only to drive that shared, already-tested code through a
// local test server, not to re-test it.
type testDiscovery struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

type testJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type testJWKSet struct {
	Keys []testJWK `json:"keys"`
}

func startOIDCServer(t *testing.T, keys testJWKSet) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(testDiscovery{Issuer: srv.URL, JWKSURI: srv.URL + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(keys)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func rsaJWK(kid string, pub *rsa.PublicKey) testJWK {
	return testJWK{
		Kty: "RSA",
		Kid: kid,
		Use: "sig",
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

func signRS256(t *testing.T, priv *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(priv)
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}
	return signed
}

const testUserUUID = "22222222-2222-2222-2222-222222222222"

func TestVerifierAcceptsValidToken(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	const kid = "test-key-1"
	srv := startOIDCServer(t, testJWKSet{Keys: []testJWK{rsaJWK(kid, &priv.PublicKey)}})

	v := NewVerifier(srv.URL, "", "", srv.Client(), logger.New())
	token := signRS256(t, priv, kid, jwt.MapClaims{
		"iss":   srv.URL,
		"sub":   testUserUUID,
		"email": "user@example.com",
		"name":  "Test User",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})

	identity, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identity.UserID != testUserUUID {
		t.Errorf("UserID = %q, want %q", identity.UserID, testUserUUID)
	}
	if identity.Provider != "simple" || identity.ExternalID != testUserUUID {
		t.Errorf("identity = %+v", identity)
	}
	if identity.Email != "user@example.com" || identity.DisplayName != "Test User" {
		t.Errorf("identity = %+v", identity)
	}
}

func TestVerifierDerivesUUIDv5ForNonUUIDSubject(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	const kid = "test-key-1"
	srv := startOIDCServer(t, testJWKSet{Keys: []testJWK{rsaJWK(kid, &priv.PublicKey)}})

	v := NewVerifier(srv.URL, "", "", srv.Client(), logger.New())
	token := signRS256(t, priv, kid, jwt.MapClaims{
		"iss": srv.URL,
		"sub": "not-a-uuid-subject",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	identity, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identity.UserID == "not-a-uuid-subject" {
		t.Fatal("UserID must be derived, not the raw non-UUID subject")
	}

	// Deterministic: verifying the same subject again must derive the
	// same user_id.
	identity2, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if identity2.UserID != identity.UserID {
		t.Errorf("deriveUserID not deterministic: %q != %q", identity.UserID, identity2.UserID)
	}
}

func TestVerifierRejectsExpiredToken(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	const kid = "test-key-1"
	srv := startOIDCServer(t, testJWKSet{Keys: []testJWK{rsaJWK(kid, &priv.PublicKey)}})

	v := NewVerifier(srv.URL, "", "", srv.Client(), logger.New())
	token := signRS256(t, priv, kid, jwt.MapClaims{
		"iss": srv.URL,
		"sub": testUserUUID,
		"exp": time.Now().Add(-time.Hour).Unix(),
	})

	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("want error for expired token, got nil")
	}
}

func TestVerifierRejectsWrongIssuer(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	const kid = "test-key-1"
	srv := startOIDCServer(t, testJWKSet{Keys: []testJWK{rsaJWK(kid, &priv.PublicKey)}})

	v := NewVerifier(srv.URL, "", "", srv.Client(), logger.New())
	token := signRS256(t, priv, kid, jwt.MapClaims{
		"iss": "https://not-the-configured-issuer.example.com",
		"sub": testUserUUID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("want error for mismatched issuer, got nil")
	}
}

func TestVerifierRejectsBadSignature(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	const kid = "test-key-1"
	srv := startOIDCServer(t, testJWKSet{Keys: []testJWK{rsaJWK(kid, &priv.PublicKey)}})

	v := NewVerifier(srv.URL, "", "", srv.Client(), logger.New())
	token := signRS256(t, other, kid, jwt.MapClaims{
		"iss": srv.URL,
		"sub": testUserUUID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("want error for bad signature, got nil")
	}
}

func TestVerifierRejectsUnknownKid(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv := startOIDCServer(t, testJWKSet{Keys: []testJWK{rsaJWK("published-key", &priv.PublicKey)}})

	v := NewVerifier(srv.URL, "", "", srv.Client(), logger.New())
	token := signRS256(t, priv, "unpublished-key", jwt.MapClaims{
		"iss": srv.URL,
		"sub": testUserUUID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("want error for unknown kid, got nil")
	}
}

func TestVerifierEnforcesConfiguredAudience(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	const kid = "test-key-1"
	srv := startOIDCServer(t, testJWKSet{Keys: []testJWK{rsaJWK(kid, &priv.PublicKey)}})

	v := NewVerifier(srv.URL, "expected-client-id", "", srv.Client(), logger.New())

	t.Run("matching audience", func(t *testing.T) {
		token := signRS256(t, priv, kid, jwt.MapClaims{
			"iss": srv.URL,
			"sub": testUserUUID,
			"aud": "expected-client-id",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		if _, err := v.Verify(context.Background(), token); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("mismatched audience", func(t *testing.T) {
		token := signRS256(t, priv, kid, jwt.MapClaims{
			"iss": srv.URL,
			"sub": testUserUUID,
			"aud": "some-other-client",
			"exp": time.Now().Add(time.Hour).Unix(),
		})
		if _, err := v.Verify(context.Background(), token); err == nil {
			t.Fatal("want error for mismatched audience, got nil")
		}
	})
}

func TestVerifierRejectsEmptyToken(t *testing.T) {
	v := NewVerifier("https://issuer.example.com", "", "https://issuer.example.com/jwks", http.DefaultClient, logger.New())
	if _, err := v.Verify(context.Background(), "   "); err == nil {
		t.Fatal("want error for empty token, got nil")
	}
}

func TestVerifierJWKSURLOverrideSkipsDiscovery(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	const kid = "test-key-1"

	// A server that only serves /jwks, never a discovery document —
	// proves the override path never calls discovery.
	mux := http.NewServeMux()
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(testJWKSet{Keys: []testJWK{rsaJWK(kid, &priv.PublicKey)}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	v := NewVerifier(srv.URL, "", srv.URL+"/jwks", srv.Client(), logger.New())
	token := signRS256(t, priv, kid, jwt.MapClaims{
		"iss": srv.URL,
		"sub": testUserUUID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, err := v.Verify(context.Background(), token); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
