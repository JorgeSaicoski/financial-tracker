package services

import "context"

// IdentityVerifier is the port the HTTP auth middleware needs from the
// outside world: verify a bearer credential and return the caller's
// user_id. Authentik (OIDC/JWT via JWKS) is the only implementation
// today — infrastructure/authentik — but the application only depends on
// this interface, so a local/dev verifier or a Google-backed one could
// satisfy it later; cmd/api/main.go would be the only place that changes
// to pick between them.
type IdentityVerifier interface {
	// Verify checks token's authenticity and returns the user_id it
	// asserts, already in financial-tracker's internal lowercase-UUID
	// form. Returns an error if the token is missing, malformed, expired,
	// or fails verification.
	Verify(ctx context.Context, token string) (userID string, err error)
}
