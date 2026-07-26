package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// UserSettingsRepository stores per-user settings (BACK-13): entitlement
// (operator/billing-controlled) vs preference (user-controlled). Absence
// of a row means "everything true" — Get must return
// dto.DefaultUserSettings rather than apperrors.ErrNotFound, so existing
// users need no backfill. A row is only created lazily, on first write
// (UpdateEnabled).
type UserSettingsRepository interface {
	// Get returns the user's settings, or the all-true defaults if no
	// row exists yet.
	Get(ctx context.Context, userID string) (*dto.UserSettingsDTO, error)
	// UpdateEnabled upserts ledgerSyncEnabled — the only field the API
	// (as opposed to an operator) is allowed to write. Entitlement
	// fields are left at their current value, or the default (true) if
	// the row didn't exist yet.
	UpdateEnabled(ctx context.Context, userID string, ledgerSyncEnabled bool) (*dto.UserSettingsDTO, error)
	// ListSyncDisabledUserIDs returns the user_ids whose effective
	// ledger sync (entitled AND enabled) is off. The sync loop excludes
	// their movements from every pass, even ones already sitting as
	// "pending" from before they turned sync off.
	ListSyncDisabledUserIDs(ctx context.Context) ([]string, error)
}
