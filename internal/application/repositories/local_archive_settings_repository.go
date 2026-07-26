package repositories

import "context"

// LocalArchiveSettingsRepository persists BACK-15's per-user
// local_archive_enabled toggle: whether the user has opted into the
// client-side-encrypted "no cloud" archive flow. A standalone table today
// (see migrations/011_add_local_archive_setting.sql) since BACK-13's
// shared user_settings row hasn't landed in main yet.
type LocalArchiveSettingsRepository interface {
	// IsEnabled reports the current setting. A user with no row yet
	// (never toggled it) defaults to false, not an error.
	IsEnabled(ctx context.Context, userID string) (bool, error)
	// SetEnabled upserts the setting. It never touches any other setting
	// (e.g. BACK-16's cloud_storage_enabled) — the two are independent
	// toggles a user combines however they want.
	SetEnabled(ctx context.Context, userID string, enabled bool) error
}
