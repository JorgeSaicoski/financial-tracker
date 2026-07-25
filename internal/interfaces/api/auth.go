package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/reqctx"
	interfacedto "github.com/JorgeSaicoski/financial-tracker/internal/interfaces/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

var errMissingAuthHeader = errors.New("missing or malformed Authorization header")

// Middleware builds an AuthMiddleware backed by a services.IdentityVerifier:
// it pulls the bearer token out of the Authorization header, verifies it,
// provisions/refreshes the local User row via ensureUser (BACK-02's
// EnsureUserUseCase), and attaches the resulting user_id to the request
// context (reqctx) for every handler downstream. It never trusts user_id
// from a request body or query string.
//
// This package doesn't know Authentik, JWT, or JWKS exist — cmd/api/main.go
// picks the concrete verifier (infrastructure/authentik today) and passes
// it in here, so swapping identity providers never touches this file.
// DevUserMiddleware is the AUTH_DISABLED alternative main.go can pick
// instead.
func Middleware(verifier services.IdentityVerifier, ensureUser usecases.EnsureUserUseCase, log logger.Logger) AuthMiddleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := bearerToken(r)
			if err != nil {
				log.Error("auth: rejecting request to %s %s: %v", r.Method, r.URL.Path, err)
				writeUnauthorized(log, w)
				return
			}
			identity, err := verifier.Verify(r.Context(), token)
			if err != nil {
				log.Error("auth: rejecting request to %s %s: %v", r.Method, r.URL.Path, err)
				writeUnauthorized(log, w)
				return
			}
			if err := ensureUserExists(r.Context(), ensureUser, identity); err != nil {
				log.Error("auth: provisioning user for request to %s %s: %v", r.Method, r.URL.Path, err)
				writeInternalError(log, w)
				return
			}
			next.ServeHTTP(w, r.WithContext(reqctx.WithUserID(r.Context(), identity.UserID)))
		})
	}
}

func bearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	// RFC 7235: the auth-scheme token ("Bearer") is case-insensitive, so
	// clients sending "bearer <token>" must still be accepted.
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", errMissingAuthHeader
	}
	return strings.TrimSpace(header[len(prefix):]), nil
}

// DevUserMiddleware is the AUTH_DISABLED=true escape hatch: every request
// is attributed to a fixed dev user id, no token required at all. It still
// runs ensureUser so DEFAULT_USER_ID has a real User row to reference, the
// same as a normal login would produce. Only meant for local development /
// single-user self-hosting — cmd/api/main.go logs loudly at startup
// whenever this is wired in instead of a real verifier-backed Middleware.
func DevUserMiddleware(userID string, ensureUser usecases.EnsureUserUseCase, log logger.Logger) func(http.Handler) http.Handler {
	identity := services.Identity{UserID: userID, Provider: "dev", ExternalID: userID, DisplayName: "Dev User"}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := ensureUserExists(r.Context(), ensureUser, identity); err != nil {
				log.Error("auth: provisioning dev user for request to %s %s: %v", r.Method, r.URL.Path, err)
				writeInternalError(log, w)
				return
			}
			next.ServeHTTP(w, r.WithContext(reqctx.WithUserID(r.Context(), userID)))
		})
	}
}

// ensureUserExists adapts an Identity to EnsureUserInput and discards the
// resulting UserDTO — callers here only need the side effect (the row
// exists / is up to date), not the row itself.
func ensureUserExists(ctx context.Context, ensureUser usecases.EnsureUserUseCase, identity services.Identity) error {
	_, err := ensureUser.Execute(ctx, usecases.EnsureUserInput{
		UserID:      identity.UserID,
		Provider:    identity.Provider,
		ExternalID:  identity.ExternalID,
		Email:       identity.Email,
		DisplayName: identity.DisplayName,
	})
	return err
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

// writeInternalError writes a 500 in the same shape as writeUnauthorized,
// for the (expected to be rare) case where the token is valid but
// provisioning the local User row failed — a database problem, not an
// auth problem.
func writeInternalError(log logger.Logger, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	if err := json.NewEncoder(w).Encode(interfacedto.ErrorResponse{Error: "internal error"}); err != nil {
		log.Error("auth: failed to encode 500 response: %v", err)
	}
}
