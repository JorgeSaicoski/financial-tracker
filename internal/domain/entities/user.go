package entities

import (
	"fmt"
	"time"
)

type User struct {
	ID          string
	Provider    string
	ExternalID  string
	Email       string
	DisplayName string
	Categories  []Category
	Assets      []Asset
	Accounts    []Account

	CreatedAt time.Time
	UpdatedAt time.Time
}

// AddCategory adds c to u's own list, enforcing maxCategories (the
// caller resolves this from LimitsRepository's "max_categories_per_user"
// row, not a hardcoded value — operator-configurable, per BACK-14
// follow-up). Contributor rights (Category.ContributorIDs) are a
// separate, uncapped concern — this only counts what's in u.Categories.
func (u *User) AddCategory(c *Category, maxCategories int) error {
	if len(u.Categories) >= maxCategories {
		return fmt.Errorf("user %s has reached the maximum number of categories", u.ID)
	}
	u.Categories = append(u.Categories, *c)
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
