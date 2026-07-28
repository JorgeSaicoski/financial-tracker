package dto

import "time"

// PatchSettingsRequest is the body for PATCH /settings. It intentionally
// has no field for either entitlement flag (ledger_sync_entitled,
// cloud_storage_entitled) — those are operator/billing-controlled, never
// user-writable. The handler decodes this with DisallowUnknownFields, so
// a request that tries to set an entitlement key (or anything else)
// fails with 400 rather than being silently ignored, satisfying BACK-13's
// "PATCH cannot modify entitlement fields" acceptance criterion at the
// decode boundary instead of needing the usecase to police it.
type PatchSettingsRequest struct {
	LedgerSyncEnabled *bool `json:"ledger_sync_enabled"`
}

// SettingsResponse is a user's settings: entitlement (operator-controlled,
// read-only here) plus preference (user-controlled). Effective ledger
// sync is ledger_sync_entitled AND ledger_sync_enabled.
// SubscriptionStatus/SubscriptionCurrentPeriodEnd (BACK-19) are omitted
// (rather than emitted as null/"") when the caller has never had a
// subscription row — a free-tier user is the common case, not an error.
type SettingsResponse struct {
	UserID               string    `json:"user_id"`
	LedgerSyncEntitled   bool      `json:"ledger_sync_entitled"`
	LedgerSyncEnabled    bool      `json:"ledger_sync_enabled"`
	CloudStorageEntitled bool      `json:"cloud_storage_entitled"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`

	SubscriptionStatus           string     `json:"subscription_status,omitempty"`
	SubscriptionCurrentPeriodEnd *time.Time `json:"subscription_current_period_end,omitempty"`
}
