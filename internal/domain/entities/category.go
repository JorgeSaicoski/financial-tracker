package entities

import (
	"fmt"
	"strings"
	"time"
)

// CategoryTransfer, CategoryIncome, and CategoryOther are the three
// system category names code branches on directly (Account.Send/Receive
// reference CategoryTransferID; getCashflowUseCase excludes
// CategoryTransfer from income/expense totals; CategoryOther is the
// well-known fallback a user's own default resolves to when they
// haven't set one). None of the three carry an AvoidabilityPercent (not
// spend, in transfer/income's case) and none can be created, renamed,
// or hidden through the category API — see IsSystemCategoryName.
const (
	CategoryTransfer = "transfer"
	CategoryIncome   = "income"
	CategoryOther    = "other"
)

// CategoryTransferID, CategoryIncomeID, and CategoryOtherID are the
// fixed ids migrations/014_movement_category_fk.sql seeds the three
// system categories with — the same literal in both the SQLite and
// Postgres migration files, specifically so every environment agrees on
// them (see that migration's own comment: it's what lets
// internal/cmd/migrate-sqlite's copyCategories skip re-inserting these
// rows rather than remapping every movement/purchase that references
// them). Account.Send/Receive reference CategoryTransferID directly —
// no name lookup needed for a row every environment already has.
const (
	CategoryTransferID = "00000000-0000-0000-0000-000000000101"
	CategoryIncomeID   = "00000000-0000-0000-0000-000000000102"
	CategoryOtherID    = "00000000-0000-0000-0000-000000000103"
)

// IsSystemCategoryName reports whether name (case-insensitive) is one
// of the three reserved category names — not spend, so no
// avoidability, and protected from create/rename/hide through the
// category API. A package function (not just a *Category method) since
// the create path needs this check before any Category exists yet —
// rejecting a brand-new category someone tries to name "transfer".
func IsSystemCategoryName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return lower == CategoryTransfer || lower == CategoryIncome || lower == CategoryOther
}

// Category is a shared, globally-visible spend label (BACK-14's
// per-user registry became global in its follow-up: "I will create
// restaurant category with 80% and offer it for whoever wants to get
// it — if someone doesn't agree they can just create a new one").
// AvoidabilityPercent (0-100, how easy this kind of spend is to skip)
// is nil only for the system categories, which aren't spend at all.
// ContributorIDs is who may edit it (rename, change
// AvoidabilityPercent) — empty for the three system categories, which
// no one can edit through the API regardless.
type Category struct {
	ID                  string
	Name                string
	AvoidabilityPercent *int
	ContributorIDs      []string
	CreatedAt           time.Time
}

// IsSystem reports whether this category is one of the three reserved
// ones (see IsSystemCategoryName).
func (c *Category) IsSystem() bool {
	return IsSystemCategoryName(c.Name)
}

// CanBeEditedBy reports whether userID may rename this category or
// change its AvoidabilityPercent. A system category's ContributorIDs is
// always empty, so this already returns false for everyone on those
// three without a separate IsSystem check — kept explicit in IsSystem
// anyway, for a clearer rejection reason than "not a contributor" when
// the real reason is "this one is reserved."
func (c *Category) CanBeEditedBy(userID string) bool {
	return contains(c.ContributorIDs, userID)
}

// UpdateAvoidabilityPercent applies a new avoidability value, enforcing
// that only a contributor may do so.
func (c *Category) UpdateAvoidabilityPercent(avoidabilityPercent *int, userID string) error {
	if !c.CanBeEditedBy(userID) {
		return fmt.Errorf("user %s is not a contributor of category %s", userID, c.ID)
	}
	c.AvoidabilityPercent = avoidabilityPercent
	return nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func AddContributors(category *Category, userIDs ...string) {
	for _, userID := range userIDs {
		if !contains(category.ContributorIDs, userID) {
			category.ContributorIDs = append(category.ContributorIDs, userID)
		}
	}
}
