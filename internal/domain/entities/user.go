package entities

import "time"

// User is the local identity every account/movement/etc. is scoped to.
// It is never created directly by a client request — EnsureUserUseCase
// provisions it automatically the first time an external identity
// (Authentik today) is seen, mirroring whatever the identity provider
// asserts about the caller. Provider/ExternalID record where that
// assertion came from, so a future second identity provider (a local
// authenticator, Google, ...) doesn't collide with or overwrite an
// Authentik-provisioned row for a different external subject.
//
// Whether this user's movements sync to ledger-service lives on
// UserSettings (BACK-13), not here — that's a separate entity precisely
// because it distinguishes entitlement (operator-controlled) from
// preference (user-controlled), which a plain bool on User can't express.
type User struct {
	ID          string
	Provider    string
	ExternalID  string
	Email       string
	DisplayName string

	CreatedAt time.Time
	UpdatedAt time.Time
}
