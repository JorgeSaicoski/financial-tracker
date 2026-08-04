// Package simpleauth implements the application layer's
// services.IdentityVerifier port (BACK-20) against any provider that
// speaks the same OIDC-like contract infrastructure/authentik already
// consumes: an issuer with a JWKS (either discoverable via
// "/.well-known/openid-configuration", or given directly), and JWTs
// carrying iss/sub/exp claims (aud optional). This is deliberately
// provider-agnostic — a future standalone username/password auth service
// (BACK-20's other, not-yet-built deliverable: a new sibling repo,
// intentionally out of scope for this change — see that ticket) can plug
// in as AUTH_PROVIDER=simple today with zero further financial-tracker
// changes, as could a second OIDC-compliant issuer of any kind.
//
// This package reuses infrastructure/authentik's Client for the HTTP/JWKS
// mechanics (OIDC discovery, JWKS fetch/parse) rather than duplicating
// jwks.go — that code is already provider-agnostic in practice (see its
// own doc comment), just currently housed under the "authentik" package
// name because Authentik was its first consumer. The JWT-parsing loop,
// key cache, and sub -> user_id derivation below are NOT shared with
// authentik.Verifier: each provider needs its own cache state and its own
// Identity.Provider value, and duplicating ~90 lines of that per adapter
// is a reasonable cost for keeping providers fully independent — a bug in
// one adapter's verification loop can't affect the other. A future
// refactor could relocate Client/jwks.go into a provider-neutral package
// (e.g. infrastructure/oidcjwks) now that a second consumer exists, but
// that's a rename/relocation of already-shipped, security-sensitive code
// best done with its own review pass, not bundled into this change.
package simpleauth

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
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/authentik"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/simpleauth/entities"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

// subjectNamespace is the fixed namespace UUID used to derive a stable
// UUIDv5 user_id from a `sub` claim that isn't already a UUID (see
// deriveUserID below). Generated once, at random, and never changed —
// changing it would silently re-map every existing non-UUID-sub user to
// a new user_id, orphaning their data. Deliberately a *different* UUID
// than authentik.Verifier's own subjectNamespace: two different
// providers deriving through the same namespace could, in principle,
// collide two different real people's raw subject strings onto the same
// internal user_id — a different namespace per provider rules that out
// even if it's astronomically unlikely in practice.
const subjectNamespace = "4a910cce-a7ab-4b68-8826-2a4baef7098f"

// jwksCacheTTL mirrors authentik.Verifier's — see that package's doc
// comment for the reasoning (avoid hammering the provider on every
// request, while still picking up a key rotation promptly).
const jwksCacheTTL = 15 * time.Minute

var allowedSigningMethods = []string{
	"RS256", "RS384", "RS512",
	"ES256", "ES384", "ES512",
	"PS256", "PS384", "PS512",
}

// Verifier implements services.IdentityVerifier the same way
// authentik.Verifier does, against a different (config-supplied) issuer.
type Verifier struct {
	client    *authentik.Client
	issuerURL string
	audience  string
	log       logger.Logger

	mu        sync.Mutex
	jwksURL   string
	keys      map[string]crypto.PublicKey
	fetchedAt time.Time
}

var _ services.IdentityVerifier = (*Verifier)(nil)

// NewVerifier builds a Verifier. jwksURLOverride, if non-empty, skips
// OIDC discovery and fetches the JWKS from that URL directly. audience
// may be empty, in which case the `aud` claim is not checked.
func NewVerifier(issuerURL, audience, jwksURLOverride string, httpClient *http.Client, log logger.Logger) *Verifier {
	return &Verifier{
		client:    authentik.NewClient(issuerURL, httpClient),
		issuerURL: issuerURL,
		audience:  audience,
		jwksURL:   jwksURLOverride,
		log:       log,
	}
}

var (
	errEmptyToken     = errors.New("empty bearer token")
	errMissingSubject = errors.New("token has no sub claim")
)

// Verify implements services.IdentityVerifier.
func (v *Verifier) Verify(ctx context.Context, token string) (services.Identity, error) {
	raw := strings.TrimSpace(token)
	if raw == "" {
		return services.Identity{}, errEmptyToken
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
		return services.Identity{}, fmt.Errorf("invalid token: %w", err)
	}
	mapClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return services.Identity{}, errors.New("unexpected claims type")
	}

	claims := entities.Claims{
		Sub:   stringClaim(mapClaims, "sub"),
		Email: stringClaim(mapClaims, "email"),
		Name:  stringClaim(mapClaims, "name"),
	}
	if claims.Sub == "" {
		return services.Identity{}, errMissingSubject
	}

	userID, err := deriveUserID(claims.Sub)
	if err != nil {
		return services.Identity{}, fmt.Errorf("deriving user_id from sub: %w", err)
	}
	return claims.ToIdentity(userID), nil
}

func stringClaim(claims jwt.MapClaims, key string) string {
	s, _ := claims[key].(string)
	return strings.TrimSpace(s)
}

// deriveUserID maps a `sub` claim to the lowercase UUID ledger-service
// (and every internal call site) requires as user_id — see
// authentik.Verifier's deriveUserID for the full design rationale this
// mirrors. A provider that already issues `sub` as a lowercase UUID (the
// natural choice for any new "simple" auth service) hits the fast path;
// anything else derives deterministically via UUIDv5 instead of failing.
func deriveUserID(sub string) (string, error) {
	lower := strings.ToLower(sub)
	if id.IsUUID(lower) {
		return lower, nil
	}
	return id.NewUUIDv5(subjectNamespace, sub)
}

// keyFunc, lookupKey, refreshKeys mirror authentik.Verifier's exactly
// (see that package for the full reasoning) — this adapter's own cache
// state (v.keys/v.fetchedAt/v.jwksURL), independent of authentik's.
func (v *Verifier) keyFunc(ctx context.Context, token *jwt.Token) (interface{}, error) {
	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("simpleauth: token header has no kid")
	}
	if key, ok := v.lookupKey(kid); ok {
		return key, nil
	}
	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}
	key, ok := v.lookupKey(kid)
	if !ok {
		return nil, fmt.Errorf("simpleauth: unknown key id %q", kid)
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
		u, err := v.client.DiscoverJWKSURL(ctx)
		if err != nil {
			return err
		}
		jwksURL = u
	}

	keys, err := v.client.FetchJWKS(ctx, jwksURL)
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
