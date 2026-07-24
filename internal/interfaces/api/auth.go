package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

var errMissingAuthHeader = errors.New("missing or malformed Authorization header")

// Middleware builds an AuthMiddleware backed by a services.IdentityVerifier:
// it pulls the bearer token out of the Authorization header, verifies it,
// and attaches the resulting user_id to the request context (reqctx) for
// every handler downstream. It never trusts user_id from a request body
// or query string.
//
// This package doesn't know Authentik, JWT, or JWKS exist — cmd/api/main.go
// picks the concrete verifier (infrastructure/authentik today) and passes
// it in here, so swapping identity providers never touches this file.
// DevUserMiddleware is the AUTH_DISABLED alternative main.go can pick
// instead.
func Middleware(verifier services.IdentityVerifier, log logger.Logger) AuthMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := bearerToken(r)
			if err != nil {
				log.Error("auth: rejecting request to %s %s: %v", r.Method, r.URL.Path, err)
				writeUnauthorized(log, w)
				return
			}
			userID, err := verifier.Verify(r.Context(), token)
			if err != nil {
				log.Error("auth: rejecting request to %s %s: %v", r.Method, r.URL.Path, err)
				writeUnauthorized(log, w)
				return
			}
			next.ServeHTTP(w, r.WithContext(reqctx.WithUserID(r.Context(), userID)))
		})
	}
}

func bearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", errMissingAuthHeader
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix)), nil
}

// DevUserMiddleware is the AUTH_DISABLED=true escape hatch: every request
// is attributed to a fixed dev user id, no token required at all. Only
// meant for local development / single-user self-hosting — cmd/api/main.go
// logs loudly at startup whenever this is wired in instead of a real
// verifier-backed Middleware.
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
