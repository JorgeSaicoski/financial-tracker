package dto

import "time"

// CategoryDTO is the application layer's representation of one category
// registry row (BACK-14; shared/global as of the BACK-14 follow-up) —
// categories are no longer a fixed enum, but an extendable, globally
// visible registry, each carrying an optional AvoidabilityPercent
// (0-100; how easy this kind of spend is to skip). nil only for the
// system categories ("transfer", "income"), which aren't spend at all
// and can't have one set.
type CategoryDTO struct {
	ID                  string
	Name                string
	AvoidabilityPercent *int
	// ContributorIDs is who may edit this category (rename, change
	// AvoidabilityPercent) — there's no separate "owner"/"creator"
	// concept, just this list. Empty for the three system-seeded
	// categories ("transfer", "income", "other"), which no one can edit
	// through the API. See CategoryRepository.IsContributor.
	ContributorIDs []string
	CreatedAt      time.Time
}
