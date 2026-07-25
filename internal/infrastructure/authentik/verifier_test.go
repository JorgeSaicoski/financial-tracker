package authentik

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

// startOIDCServer spins up a discovery + JWKS endpoint serving the given
// keys, with the discovery document's issuer set to the server's own URL
// (matching how a real, correctly configured OIDC provider behaves).
func startOIDCServer(t *testing.T, keys jwkSet) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(oidcDiscovery{Issuer: srv.URL, JWKSURI: srv.URL + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(keys)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func rsaJWK(kid string, pub *rsa.PublicKey) jwk {
	return jwk{
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

const testUserUUID = "11111111-1111-1111-1111-111111111111"

func TestVerifierAcceptsValidToken(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	const kid = "test-key-1"
	srv := startOIDCServer(t, jwkSet{Keys: []jwk{rsaJWK(kid, &priv.PublicKey)}})

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
	if identity.Provider != "authentik" || identity.ExternalID != testUserUUID {
		t.Errorf("identity = %+v", identity)
	}
	if identity.Email != "user@example.com" || identity.DisplayName != "Test User" {
		t.Errorf("identity = %+v", identity)
	}
}

func TestVerifierRejectsExpiredToken(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	const kid = "test-key-1"
	srv := startOIDCServer(t, jwkSet{Keys: []jwk{rsaJWK(kid, &priv.PublicKey)}})

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
	srv := startOIDCServer(t, jwkSet{Keys: []jwk{rsaJWK(kid, &priv.PublicKey)}})

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
	// Publish priv's public key, but sign with a different private key —
	// the signature must not verify even though the kid matches.
	srv := startOIDCServer(t, jwkSet{Keys: []jwk{rsaJWK(kid, &priv.PublicKey)}})

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
	srv := startOIDCServer(t, jwkSet{Keys: []jwk{rsaJWK("published-key", &priv.PublicKey)}})

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
	srv := startOIDCServer(t, jwkSet{Keys: []jwk{rsaJWK(kid, &priv.PublicKey)}})

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

func TestVerifierSkipsNonSigningKeys(t *testing.T) {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)
	const kid = "enc-key"
	encKey := rsaJWK(kid, &priv.PublicKey)
	encKey.Use = "enc"
	srv := startOIDCServer(t, jwkSet{Keys: []jwk{encKey}})

	v := NewVerifier(srv.URL, "", "", srv.Client(), logger.New())
	token := signRS256(t, priv, kid, jwt.MapClaims{
		"iss": srv.URL,
		"sub": testUserUUID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatal("want error: encryption-only key must not be trusted for signature verification")
	}
}

func TestDiscoverJWKSURLRejectsIssuerMismatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(oidcDiscovery{Issuer: "https://evil.example.com", JWKSURI: "https://evil.example.com/jwks"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	_, err := discoverJWKSURL(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("want error when discovery document's issuer doesn't match the configured issuer")
	}
}
