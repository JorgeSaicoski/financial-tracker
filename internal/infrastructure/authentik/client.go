package authentik

import (
	"context"
	"crypto"
	"net/http"
)

// Client is the only thing in this package that knows how to talk to
// Authentik (or any spec-compliant OIDC issuer) over HTTP: OIDC discovery
// and JWKS fetch/parse. It has no notion of JWT verification or
// financial-tracker's user model — that's Verifier's job, built on top of
// Client the same way ledgerservice.gateway is built on top of
// ledgerservice.Client.
type Client struct {
	issuerURL  string
	httpClient *http.Client
}

// NewClient builds a Client against issuerURL. httpClient defaults to
// http.DefaultClient if nil.
func NewClient(issuerURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{issuerURL: issuerURL, httpClient: httpClient}
}

// DiscoverJWKSURL fetches the issuer's OIDC discovery document and
// returns its jwks_uri.
func (c *Client) DiscoverJWKSURL(ctx context.Context) (string, error) {
	return discoverJWKSURL(ctx, c.httpClient, c.issuerURL)
}

// FetchJWKS retrieves and parses jwksURL's JWKS document, returning each
// signing key indexed by its kid.
func (c *Client) FetchJWKS(ctx context.Context, jwksURL string) (map[string]crypto.PublicKey, error) {
	return fetchJWKS(ctx, c.httpClient, jwksURL)
}
