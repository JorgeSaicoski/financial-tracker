package dto

import "time"

// UserSettingsDTO is the application layer's representation of a user's
// per-user settings row (BACK-13). Entitled fields are operator/billing
// controlled; Enabled fields are user-controlled preference. Effective
// capability is Entitled AND Enabled.
type UserSettingsDTO struct {
	UserID               string
	LedgerSyncEntitled   bool
	LedgerSyncEnabled    bool
	CloudStorageEntitled bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// EffectiveLedgerSync reports whether this user's movements should
// actually be pushed to ledger-service right now.
func (s *UserSettingsDTO) EffectiveLedgerSync() bool {
	return s.LedgerSyncEntitled && s.LedgerSyncEnabled
}

// DefaultUserSettings is what an absent row means: current behavior
// preserved for every existing user, no backfill required.
func DefaultUserSettings(userID string, at time.Time) *UserSettingsDTO {
	return &UserSettingsDTO{
		UserID:               userID,
		LedgerSyncEntitled:   true,
		LedgerSyncEnabled:    true,
		CloudStorageEntitled: true,
		CreatedAt:            at,
		UpdatedAt:            at,
	}
}
