// Package entities holds the "simple" JWT provider's wire-format claim
// shape, private to the simpleauth infrastructure package. It converts to
// the application layer's services.Identity via ToIdentity() — the same
// pattern infrastructure/authentik/entities and
// infrastructure/ledgerservice/entities use. Application and domain code
// never see Claims.
package entities

import "github.com/JorgeSaicoski/financial-tracker/internal/application/services"

// Claims is the subset of a verified token's claims this package maps
// into a local User: the same OIDC-like iss/sub/exp/aud shape
// infrastructure/authentik consumes, so any provider that speaks it (a
// future standalone username/password auth service, another OIDC issuer,
// ...) can plug in as AUTH_PROVIDER=simple with zero financial-tracker
// code changes — see cmd/api/main.go. Sub is the only field this package
// requires; Email/Name may be empty for a minimal provider that doesn't
// populate profile claims.
type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// ToIdentity maps this package's claim shape to the application layer's
// Identity. userID is passed in rather than derived here — the sub ->
// UUID derivation (deriveUserID) stays in verifier.go, this function
// only knows about field mapping.
func (c Claims) ToIdentity(userID string) services.Identity {
	return services.Identity{
		UserID:      userID,
		Provider:    "simple",
		ExternalID:  c.Sub,
		Email:       c.Email,
		DisplayName: c.Name,
	}
}
