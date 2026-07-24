// Package authentik implements the application layer's
// services.IdentityVerifier port against Authentik (or any other
// spec-compliant OIDC provider): it discovers/fetches a JWKS, verifies a
// JWT's signature/iss/exp/aud, and maps the token's `sub` claim to
// financial-tracker's internal lowercase-UUID user_id. This is the only
// package that knows OIDC/JWT/JWKS/Authentik exist — everything else
// (interfaces/api's auth middleware, cmd/api's wiring) depends only on
// services.IdentityVerifier, so swapping in a different identity provider
// later means writing a new package like this one, not touching the
// interfaces or application layers.
package authentik

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

// subjectNamespace is the fixed namespace UUID used to derive a stable
// UUIDv5 user_id from an OIDC `sub` claim that isn't already a UUID (see
// deriveUserID below). Generated once, at random, and never changed —
// changing it would silently re-map every existing non-UUID-sub user to a
// new user_id, orphaning their data.
const subjectNamespace = "64b72193-8b2a-484c-9728-6b703fa90ca9"

// jwksCacheTTL bounds how long a fetched JWKS is trusted before the next
// verification forces a refetch — long enough to avoid hammering the
// identity provider on every request, short enough that a key rotation or
// revocation takes effect promptly.
const jwksCacheTTL = 15 * time.Minute

var allowedSigningMethods = []string{
	"RS256", "RS384", "RS512",
	"ES256", "ES384", "ES512",
	"PS256", "PS384", "PS512",
}

// Verifier implements services.IdentityVerifier by validating a JWT
// against an OIDC issuer's published JWKS: signature, iss, exp (both via
// jwt/v5's default validation) and aud (when audience is configured).
type Verifier struct {
	issuerURL  string
	audience   string
	httpClient *http.Client
	log        logger.Logger

	mu        sync.Mutex
	jwksURL   string // resolved once (via discovery or jwksURLOverride), then reused
	keys      map[string]crypto.PublicKey
	fetchedAt time.Time
}

var _ services.IdentityVerifier = (*Verifier)(nil)

// NewVerifier builds a Verifier. jwksURLOverride, if non-empty, skips OIDC
// discovery and fetches the JWKS from that URL directly — useful for
// tests and for providers whose discovery document is unreachable for
// some reason. audience may be empty, in which case the `aud` claim is
// not checked (see cmd/api/main.go's OIDC_AUDIENCE handling for why
// that's still safe in this deployment's default configuration).
func NewVerifier(issuerURL, audience, jwksURLOverride string, httpClient *http.Client, log logger.Logger) *Verifier {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Verifier{
		issuerURL:  issuerURL,
		audience:   audience,
		jwksURL:    jwksURLOverride,
		httpClient: httpClient,
		log:        log,
	}
}

var (
	errEmptyToken     = errors.New("empty bearer token")
	errMissingSubject = errors.New("token has no sub claim")
)

// Verify implements services.IdentityVerifier.
func (v *Verifier) Verify(ctx context.Context, token string) (string, error) {
	raw := strings.TrimSpace(token)
	if raw == "" {
		return "", errEmptyToken
	}

	opts := []jwt.ParserOption{
		jwt.WithValidMethods(allowedSigningMethods),
		jwt.WithIssuer(v.issuerURL),
		jwt.WithExpirationRequired(),
	}
	if v.audience != "" {
		opts = append(opts, jwt.WithAudience(v.audience))
	}

	parsed, err := jwt.Parse(raw, func(t *jwt.Token) (interface{}, error) {
		return v.keyFunc(ctx, t)
	}, opts...)
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("unexpected claims type")
	}
	sub, _ := claims["sub"].(string)
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return "", errMissingSubject
	}

	userID, err := deriveUserID(sub)
	if err != nil {
		return "", fmt.Errorf("deriving user_id from sub: %w", err)
	}
	return userID, nil
}

// deriveUserID maps an OIDC `sub` claim to the lowercase UUID
// ledger-service (and every internal call site) requires as user_id.
//
// Design decision (documented here since the ticket left it open): this
// deployment's Authentik provider is configured with `sub_mode:
// user_uuid` (see deploy/authentik/blueprints/financial-tracker.yaml), so
// in practice `sub` already *is* the user's lowercase UUID and this
// function is the identity mapping (after a defensive lowercase). The
// UUIDv5 fallback below exists for robustness, not because today's
// deployment needs it: a differently-configured Authentik instance, a
// future non-Authentik OIDC provider, or a subject format change
// upstream would otherwise turn into a hard failure for every user
// instead of degrading gracefully. UUIDv5 (namespace + name, SHA-1) was
// chosen over a random/local mapping table because it's deterministic and
// needs no persisted state: the same external identity always derives
// the same local user_id on every request, from any financial-tracker
// instance, with nothing to keep in sync.
func deriveUserID(sub string) (string, error) {
	lower := strings.ToLower(sub)
	if id.IsUUID(lower) {
		return lower, nil
	}
	return id.NewUUIDv5(subjectNamespace, sub)
}

// keyFunc implements jwt.Keyfunc (via the closure in Verify): look up the
// token's kid in the cached JWKS, refreshing once if it's missing (covers
// both an empty initial cache and a key rotation the TTL hasn't caught
// yet). ctx is Verify's caller-supplied context, so a refresh respects
// the request's cancellation/deadline instead of blocking indefinitely.
func (v *Verifier) keyFunc(ctx context.Context, token *jwt.Token) (interface{}, error) {
	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("authentik: token header has no kid")
	}
	if key, ok := v.lookupKey(kid); ok {
		return key, nil
	}
	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}
	key, ok := v.lookupKey(kid)
	if !ok {
		return nil, fmt.Errorf("authentik: unknown key id %q", kid)
	}
	return key, nil
}

func (v *Verifier) lookupKey(kid string) (crypto.PublicKey, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.keys == nil || time.Since(v.fetchedAt) > jwksCacheTTL {
		return nil, false
	}
	k, ok := v.keys[kid]
	return k, ok
}

func (v *Verifier) refreshKeys(ctx context.Context) error {
	v.mu.Lock()
	jwksURL := v.jwksURL
	v.mu.Unlock()

	if jwksURL == "" {
		u, err := discoverJWKSURL(ctx, v.httpClient, v.issuerURL)
		if err != nil {
			return err
		}
		jwksURL = u
	}

	keys, err := fetchJWKS(ctx, v.httpClient, jwksURL)
	if err != nil {
		return err
	}

	v.mu.Lock()
	v.jwksURL = jwksURL
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}
