package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
)

// MovementRepository is the contract the domain depends on to persist and
// query movements. It is implemented today by infrastructure/ledgerservice
// (calls out to ledger-service over HTTP) and, later, by an
// infrastructure/postgresql implementation — usecases and handlers never
// need to change when that swap happens.
type MovementRepository interface {
	Create(ctx context.Context, movement *entities.Movement) (*entities.Movement, error)
	GetByID(ctx context.Context, id string) (*entities.Movement, error)
	ListByUser(ctx context.Context, userID string, currency *string, limit, offset int) ([]*entities.Movement, error)
}
