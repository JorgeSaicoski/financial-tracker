package api

import (
	"context"
	"crypto"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
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

// Authenticator validates `Authorization: Bearer <jwt>` against an OIDC
// issuer's published JWKS: signature, iss, exp (both via jwt/v5's default
// validation) and aud (when Audience is configured). It never trusts
// user_id from a request body or query string — the only user_id a
// request can produce is the one derived from the token's verified `sub`
// claim, attached to the context via reqctx.
type Authenticator struct {
	issuerURL  string
	audience   string
	httpClient *http.Client
	log        logger.Logger

	mu        sync.Mutex
	jwksURL   string // resolved once (via discovery or JWKSURLOverride), then reused
	keys      map[string]crypto.PublicKey
	fetchedAt time.Time
}

// NewAuthenticator builds an Authenticator. jwksURLOverride, if non-empty,
// skips OIDC discovery and fetches the JWKS from that URL directly —
// useful for tests and for providers whose discovery document is
// unreachable for some reason. audience may be empty, in which case the
// `aud` claim is not checked (see cmd/api/main.go's OIDC_AUDIENCE
// handling for why that's still safe in this deployment's default
// configuration).
func NewAuthenticator(issuerURL, audience, jwksURLOverride string, httpClient *http.Client, log logger.Logger) *Authenticator {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Authenticator{
		issuerURL:  issuerURL,
		audience:   audience,
		jwksURL:    jwksURLOverride,
		httpClient: httpClient,
		log:        log,
	}
}

// Middleware rejects any request without a valid bearer token with 401 +
// the standard ErrorResponse shape; a valid token gets its derived
// user_id attached to the request context for every handler downstream.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, err := a.authenticate(r)
		if err != nil {
			a.log.Error("auth: rejecting request to %s %s: %v", r.Method, r.URL.Path, err)
			writeUnauthorized(a.log, w)
			return
		}
		next.ServeHTTP(w, r.WithContext(reqctx.WithUserID(r.Context(), userID)))
	})
}

var (
	errMissingAuthHeader = errors.New("missing or malformed Authorization header")
	errEmptyToken        = errors.New("empty bearer token")
	errMissingSubject    = errors.New("token has no sub claim")
)

func (a *Authenticator) authenticate(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", errMissingAuthHeader
	}
	raw := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if raw == "" {
		return "", errEmptyToken
	}

	opts := []jwt.ParserOption{
		jwt.WithValidMethods(allowedSigningMethods),
		jwt.WithIssuer(a.issuerURL),
		jwt.WithExpirationRequired(),
	}
	if a.audience != "" {
		opts = append(opts, jwt.WithAudience(a.audience))
	}

	token, err := jwt.Parse(raw, a.keyFunc, opts...)
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
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

// keyFunc implements jwt.Keyfunc: look up the token's kid in the cached
// JWKS, refreshing once if it's missing (covers both an empty initial
// cache and a key rotation the TTL hasn't caught yet).
func (a *Authenticator) keyFunc(token *jwt.Token) (interface{}, error) {
	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		return nil, errors.New("auth: token header has no kid")
	}
	if key, ok := a.lookupKey(kid); ok {
		return key, nil
	}
	if err := a.refreshKeys(context.Background()); err != nil {
		return nil, err
	}
	key, ok := a.lookupKey(kid)
	if !ok {
		return nil, fmt.Errorf("auth: unknown key id %q", kid)
	}
	return key, nil
}

func (a *Authenticator) lookupKey(kid string) (crypto.PublicKey, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.keys == nil || time.Since(a.fetchedAt) > jwksCacheTTL {
		return nil, false
	}
	k, ok := a.keys[kid]
	return k, ok
}

func (a *Authenticator) refreshKeys(ctx context.Context) error {
	a.mu.Lock()
	jwksURL := a.jwksURL
	a.mu.Unlock()

	if jwksURL == "" {
		u, err := discoverJWKSURL(ctx, a.httpClient, a.issuerURL)
		if err != nil {
			return err
		}
		jwksURL = u
	}

	keys, err := fetchJWKS(ctx, a.httpClient, jwksURL)
	if err != nil {
		return err
	}

	a.mu.Lock()
	a.jwksURL = jwksURL
	a.keys = keys
	a.fetchedAt = time.Now()
	a.mu.Unlock()
	return nil
}

// DevUserMiddleware is the AUTH_DISABLED=true escape hatch: every request
// is attributed to a fixed dev user id, no token required at all. Only
// meant for local development / single-user self-hosting — cmd/api/main.go
// logs loudly at startup whenever this is wired in instead of a real
// Authenticator.
func DevUserMiddleware(userID string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(reqctx.WithUserID(r.Context(), userID)))
		})
	}
}

// writeUnauthorized writes a 401 in the same interfacedto.ErrorResponse
// shape every other handler error uses (handlers/http_helpers.go's
// writeError). Duplicated rather than imported: this package (api)
// already imports handlers to wire routes in router.go, so handlers
// importing back here — even just for this helper — would be a cycle.
func writeUnauthorized(log logger.Logger, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	if err := json.NewEncoder(w).Encode(interfacedto.ErrorResponse{Error: "unauthorized"}); err != nil {
		log.Error("auth: failed to encode 401 response: %v", err)
	}
}
