package repositories

import "context"

// LimitsRepository reads operator-configurable numeric limits (BACK-14
// follow-up: "max_categories_per_user" is the first one) — a name/
// description/value row per limit, so retuning one is a database update,
// not a code change/redeploy. Read-only from the application's side; no
// API writes it today.
type LimitsRepository interface {
	// GetValue returns the current value of the named limit.
	// apperrors.ErrNotFound if name isn't a recognized limit.
	GetValue(ctx context.Context, name string) (int, error)
}
