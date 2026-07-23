package repositories

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
)

// ExchangeRateRepository persists user-entered, per-user, historical
// exchange rates against USD, expressed in application/dto types.
type ExchangeRateRepository interface {
	// Create inserts a rate row, generating its ID. A row already present
	// for the same (UserID, Currency, EffectiveFrom) has its UnitsPerUSD
	// replaced in place instead (its ID and original CreatedAt are kept)
	// — this is what makes posting the same effective_date twice a
	// backfill correction, not a duplicate.
	Create(ctx context.Context, rate *dto.ExchangeRateDTO) (*dto.ExchangeRateDTO, error)
	// ListByUser returns every rate row for the user, across all
	// currencies, newest EffectiveFrom first.
	ListByUser(ctx context.Context, userID string) ([]*dto.ExchangeRateDTO, error)
	// RateAt returns the row with the greatest EffectiveFrom <= at for
	// (userID, currency). apperrors.ErrNotFound if none exists.
	RateAt(ctx context.Context, userID, currency string, at time.Time) (*dto.ExchangeRateDTO, error)
	// Delete removes a rate row owned by userID. apperrors.ErrNotFound if
	// it doesn't exist or isn't owned by userID — fixing a typo in
	// history is legitimate, this is user-owned reference data.
	Delete(ctx context.Context, userID, id string) error
}
