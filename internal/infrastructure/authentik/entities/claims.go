// Package entities holds Authentik's wire-format JWT claim shape,
// private to the authentik infrastructure package. It converts to the
// application layer's services.Identity via ToIdentity() — "infrastructure
// adapts to the application's contract", the same pattern
// infrastructure/ledgerservice/entities uses for ledger-service's
// transaction shape. Application and domain code never see Claims.
package entities

import "github.com/JorgeSaicoski/financial-tracker/internal/application/services"

// Claims is the subset of a verified Authentik JWT's claims this package
// maps into a local User: subject plus whatever profile fields the
// deployment's OIDC scopes (openid email profile) put on the token. Any
// of these besides Sub may be empty — Authentik populates them from
// scopes, and this package tolerates a differently-scoped or
// differently-configured issuer degrading gracefully rather than failing
// verification over a missing display name.
type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// ToIdentity maps Authentik's claim shape to the application layer's
// Identity. userID is passed in rather than derived here — the sub → UUID
// derivation (deriveUserID) stays in verifier.go, this function only
// knows about field mapping.
func (c Claims) ToIdentity(userID string) services.Identity {
	return services.Identity{
		UserID:      userID,
		Provider:    "authentik",
		ExternalID:  c.Sub,
		Email:       c.Email,
		DisplayName: c.Name,
	}
}
