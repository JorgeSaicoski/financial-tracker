package usecases

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	"github.com/JorgeSaicoski/financial-tracker/domain/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

// ListMovementsResult also carries the computed balance, since
// ledger-service deliberately leaves that calculation to consumers.
type ListMovementsResult struct {
	Movements []*entities.Movement
	Balance   int64
}

type ListMovementsUseCase interface {
	Execute(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) (ListMovementsResult, error)
}

type listMovementsUseCase struct {
	repo repositories.MovementRepository
}

// NewListMovements returns interface type for dependency injection.
func NewListMovements(repo repositories.MovementRepository) ListMovementsUseCase {
	return &listMovementsUseCase{repo: repo}
}

func (uc *listMovementsUseCase) Execute(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) (ListMovementsResult, error) {
	if userID == "" {
		return ListMovementsResult{}, apperrors.ErrInvalidInput
	}
	if limit < 0 || offset < 0 {
		return ListMovementsResult{}, apperrors.ErrInvalidInput
	}
	if from != nil && to != nil && !from.Before(*to) {
		return ListMovementsResult{}, apperrors.ErrInvalidInput
	}

	movements, err := uc.repo.ListByUser(ctx, userID, currency, from, to, limit, offset)
	if err != nil {
		return ListMovementsResult{}, err
	}

	// Voided movements were cancelled before ever reaching ledger-service
	// — they count as if they never happened. A synced-then-cancelled
	// movement and its reversal are both active with opposite amounts, so
	// they net to zero here without special-casing, exactly as
	// ledger-service's own records would.
	var balance int64
	for _, m := range movements {
		if m.Status == entities.MovementStatusVoided {
			continue
		}
		balance += m.Amount
	}

	return ListMovementsResult{Movements: movements, Balance: balance}, nil
}
