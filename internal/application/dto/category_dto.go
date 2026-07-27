package dto

import "time"

// CategoryDTO is the application layer's representation of one user's
// category registry row (BACK-14) — categories are no longer a fixed
// enum, but a per-user registry, each carrying an optional
// AvoidabilityPercent (0-100; how easy this kind of spend is to skip).
// nil only for the two system categories ("transfer", "income"), which
// aren't spend at all and can't have one set.
type CategoryDTO struct {
	ID                  string
	UserID              string
	Name                string
	AvoidabilityPercent *int
	// IsDefault (BACK-14 follow-up) marks the one category per user that
	// movements/purchases get reassigned to when their own category is
	// deleted — exactly one per user, enforced by a partial unique index
	// (see migrations/014_movement_category_fk.sql).
	IsDefault bool
	CreatedAt time.Time
}
