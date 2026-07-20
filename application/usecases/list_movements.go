package usecases

import (
	"context"

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
	Execute(ctx context.Context, userID string, currency *string, limit, offset int) (ListMovementsResult, error)
}

type listMovementsUseCase struct {
	repo repositories.MovementRepository
}

// NewListMovements returns interface type for dependency injection.
func NewListMovements(repo repositories.MovementRepository) ListMovementsUseCase {
	return &listMovementsUseCase{repo: repo}
}

func (uc *listMovementsUseCase) Execute(ctx context.Context, userID string, currency *string, limit, offset int) (ListMovementsResult, error) {
	if userID == "" {
		return ListMovementsResult{}, apperrors.ErrInvalidInput
	}
	if limit < 0 || offset < 0 {
		return ListMovementsResult{}, apperrors.ErrInvalidInput
	}

	movements, err := uc.repo.ListByUser(ctx, userID, currency, limit, offset)
	if err != nil {
		return ListMovementsResult{}, err
	}

	var balance int64
	for _, m := range movements {
		balance += m.Amount
	}

	return ListMovementsResult{Movements: movements, Balance: balance}, nil
}
