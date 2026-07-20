package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
)

// MovementRepository is the contract the application core depends on to
// persist and query movements. It is implemented today by
// infrastructure/ledgerservice (calls out to ledger-service over HTTP) and,
// later, by an infrastructure/postgresql implementation — usecases and
// handlers never need to change when that swap happens.
type MovementRepository interface {
	Create(ctx context.Context, input dto.CreateMovementInput) (dto.MovementOutput, error)
	GetByID(ctx context.Context, id string) (dto.MovementOutput, error)
	ListByUser(ctx context.Context, userID string, currency *string, limit, offset int) ([]dto.MovementOutput, error)
}
