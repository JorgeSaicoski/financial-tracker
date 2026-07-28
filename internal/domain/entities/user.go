package entities

import (
	"fmt"
	"time"
)

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
// Same reasoning for DefaultCategoryID (BACK-14 follow-up): it lives on
// UserSettings too, not here — a single indexed FK lookup, no need to
// carry this user's whole category list around to answer "which one is
// the default."
type User struct {
	ID          string
	Provider    string
	ExternalID  string
	Email       string
	DisplayName string
	Categories  []*Category

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (u *User) AddCategory(c *Category) error {
	if len(u.Categories) >= 10 { //change to get it from db
		return fmt.Errorf("user %s has reached the maximum number of categories", u.ID)
	}
	u.Categories = append(u.Categories, c)
	return nil
}

func (u *User) RemoveCategory(c *Category) error {
	for i, cat := range u.Categories {
		if cat.ID == c.ID {
			u.Categories = append(u.Categories[:i], u.Categories[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("category %s not found for user %s", c.ID, u.ID)
}
