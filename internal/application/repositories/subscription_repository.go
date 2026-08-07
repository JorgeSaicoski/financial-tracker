package repositories

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// SubscriptionRepository persists each user's current subscription row
// (BACK-19), expressed in application/dto types.
type SubscriptionRepository interface {
	// Upsert inserts or replaces the caller's current subscription — one
	// row per user, keyed on UserID, so a resubscribe after a prior
	// cancellation overwrites rather than accumulates history.
	Upsert(ctx context.Context, sub *dto.SubscriptionDTO) (*dto.SubscriptionDTO, error)
	// GetByUserID returns apperrors.ErrNotFound if the user has never had
	// a subscription row (the common case for the free tier).
	GetByUserID(ctx context.Context, userID string) (*dto.SubscriptionDTO, error)
	// ListLapsable returns every subscription marked past_due or canceled
	// whose grace period has elapsed as of asOf — CurrentPeriodEnd +
	// graceDays <= asOf. Neither status flips cloud_storage_entitled
	// immediately on its own webhook (BACK-19: even an explicit
	// cancellation "flips it back after the grace period, not
	// immediately") — these are the billing sweep's candidates for that
	// eventual entitlement lapse (internal/application/billing).
	ListLapsable(ctx context.Context, asOf time.Time, graceDays int) ([]*dto.SubscriptionDTO, error)
}
