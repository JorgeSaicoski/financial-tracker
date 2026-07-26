package authentik

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
)

// oidcDiscovery is the subset of a standard OIDC discovery document
// (issuer + "/.well-known/openid-configuration") this package needs —
// just enough to locate the JWKS endpoint without hardcoding any
// provider-specific URL convention, so this works against Authentik today
// and any other spec-compliant OIDC issuer later.
type oidcDiscovery struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

// jwk is one entry of a JSON Web Key Set (RFC 7517), narrowed to the
// fields RSA and EC public keys use — the only key types Authentik (and
// OIDC providers generally) issue for token signing.
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`

	// RSA
	N string `json:"n"`
	E string `json:"e"`

	// EC
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type jwkSet struct {
	Keys []jwk `json:"keys"`
}

// discoverJWKSURL fetches issuerURL's OIDC discovery document and returns
// its jwks_uri, after checking the document's own issuer matches — a
// misconfigured discovery endpoint returning a different issuer's document
// would otherwise silently make this code trust the wrong provider's keys.
func discoverJWKSURL(ctx context.Context, client *http.Client, issuerURL string) (string, error) {
	discoveryURL := strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"

	body, err := getBody(ctx, client, discoveryURL)
	if err != nil {
		return "", fmt.Errorf("authentik: fetching OIDC discovery document %s: %w", discoveryURL, err)
	}
	var doc oidcDiscovery
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("authentik: parsing OIDC discovery document %s: %w", discoveryURL, err)
	}
	if doc.JWKSURI == "" {
		return "", fmt.Errorf("authentik: OIDC discovery document %s has no jwks_uri", discoveryURL)
	}
	if strings.TrimRight(doc.Issuer, "/") != strings.TrimRight(issuerURL, "/") {
		return "", fmt.Errorf("authentik: OIDC discovery document %s has issuer %q, want %q",
			discoveryURL, doc.Issuer, issuerURL)
	}
	return doc.JWKSURI, nil
}

// fetchJWKS retrieves and parses a JWKS document, returning each key
// indexed by its kid. Keys of an unsupported kty are skipped rather than
// failing the whole fetch — a provider can publish key types this package
// doesn't need to understand (e.g. an encryption key alongside signing
// keys) without breaking verification of the ones it does.
func fetchJWKS(ctx context.Context, client *http.Client, jwksURL string) (map[string]crypto.PublicKey, error) {
	body, err := getBody(ctx, client, jwksURL)
	if err != nil {
		return nil, fmt.Errorf("authentik: fetching JWKS %s: %w", jwksURL, err)
	}
	var set jwkSet
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, fmt.Errorf("authentik: parsing JWKS %s: %w", jwksURL, err)
	}

	keys := make(map[string]crypto.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kid == "" {
			continue
		}
		if k.Use != "" && k.Use != "sig" {
			// Not a signing key (e.g. "enc") — never trust it for JWT
			// verification, even if a provider publishes it in the same set.
			continue
		}
		switch k.Kty {
		case "RSA":
			pub, err := parseRSAKey(k)
			if err != nil {
				return nil, fmt.Errorf("authentik: JWKS %s: key %q: %w", jwksURL, k.Kid, err)
			}
			keys[k.Kid] = pub
		case "EC":
			pub, err := parseECKey(k)
			if err != nil {
				return nil, fmt.Errorf("authentik: JWKS %s: key %q: %w", jwksURL, k.Kid, err)
			}
			keys[k.Kid] = pub
		default:
			// Unsupported/irrelevant key type (e.g. "oct", "OKP") — skip.
		}
	}
	return keys, nil
}

func parseRSAKey(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("invalid RSA modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("invalid RSA exponent: %w", err)
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e == 0 {
		return nil, fmt.Errorf("invalid RSA exponent: zero")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

func parseECKey(k jwk) (*ecdsa.PublicKey, error) {
	var curve elliptic.Curve
	switch k.Crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported EC curve %q", k.Crv)
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf("invalid EC x coordinate: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, fmt.Errorf("invalid EC y coordinate: %w", err)
	}
	x, y := new(big.Int).SetBytes(xBytes), new(big.Int).SetBytes(yBytes)
	if !curve.IsOnCurve(x, y) {
		return nil, fmt.Errorf("EC point (x,y) is not on curve %q", k.Crv)
	}
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

// maxDiscoveryBodyBytes bounds how much of a discovery/JWKS response this
// package will read — both documents are small JSON blobs in practice, so
// this is generous headroom, not a tight fit; it exists only to stop a
// misconfigured or malicious endpoint from exhausting memory via an
// unbounded (or streamed-forever) response body.
const maxDiscoveryBodyBytes = 1 << 20 // 1 MiB

func getBody(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiscoveryBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxDiscoveryBodyBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxDiscoveryBodyBytes)
	}
	return body, nil
}
