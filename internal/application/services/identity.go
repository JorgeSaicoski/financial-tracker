package services

import "context"

// Identity is what a verified credential asserts about the caller — the
// derived user_id plus enough profile data (provider/external_id/email/
// display name) for EnsureUserUseCase to provision or refresh a local
// User row without the application layer knowing anything about OIDC,
// JWTs, or Authentik.
type Identity struct {
	// UserID is already in financial-tracker's internal lowercase-UUID
	// form (see infrastructure/authentik's deriveUserID).
	UserID string
	// Provider names the identity provider that verified this token
	// (e.g. "authentik") — recorded so a future second provider can't
	// collide with a different provider's external ID.
	Provider   string
	ExternalID string // the provider's own subject/user id (OIDC `sub`)

	Email       string
	DisplayName string
}

// IdentityVerifier is the port the HTTP auth middleware needs from the
// outside world: verify a bearer credential and return what it asserts
// about the caller. Authentik (OIDC/JWT via JWKS) is the only
// implementation today — infrastructure/authentik — but the application
// only depends on this interface, so a local/dev verifier or a
// Google-backed one could satisfy it later; cmd/api/main.go would be the
// only place that changes to pick between them.
type IdentityVerifier interface {
	// Verify checks token's authenticity and returns the Identity it
	// asserts. Returns an error if the token is missing, malformed,
	// expired, or fails verification.
	Verify(ctx context.Context, token string) (Identity, error)
}
