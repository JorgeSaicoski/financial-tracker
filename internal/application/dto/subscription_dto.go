package dto

import "time"

// Subscription status values (BACK-19). Like currencies and exchange
// rates, a subscription has no single-entity business rule worth a
// domain entity — this DTO is the whole shape, application layer through
// infrastructure.
const (
	SubscriptionStatusActive   = "active"
	SubscriptionStatusPastDue  = "past_due"
	SubscriptionStatusCanceled = "canceled"
)

// ValidSubscriptionStatus reports whether s is one of the three statuses
// the payment provider can report.
func ValidSubscriptionStatus(s string) bool {
	switch s {
	case SubscriptionStatusActive, SubscriptionStatusPastDue, SubscriptionStatusCanceled:
		return true
	}
	return false
}

// SubscriptionDTO is the application layer's representation of a user's
// current subscription row — the provider's view of billing state, kept
// separate from user_settings.cloud_storage_entitled (the flag the rest
// of the app actually reads). One row per user: a new subscription for
// an already-subscribed user replaces the old row, mirroring
// UserSettingsDTO's "current state, not history" shape.
type SubscriptionDTO struct {
	UserID                 string
	Provider               string
	ProviderSubscriptionID string
	Status                 string // active | past_due | canceled
	CurrentPeriodEnd       time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}
