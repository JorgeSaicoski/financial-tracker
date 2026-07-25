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
type User struct {
	ID          string
	Provider    string
	ExternalID  string
	Email       string
	DisplayName string

	// CloudSyncEnabled gates whether this user's movements sync to
	// ledger-service. Not wired to any use case yet — a future ticket
	// decides how a user turns cloud sync on/off; the field exists now so
	// the column and domain concept are in place ahead of that.
	CloudSyncEnabled bool

	CreatedAt time.Time
	UpdatedAt time.Time
}
