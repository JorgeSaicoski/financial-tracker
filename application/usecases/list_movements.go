package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

// ListMovements also computes the running balance (sum of amounts) for the
// filtered set, since ledger-service deliberately leaves that to consumers.
type ListMovements struct {
	repo repositories.MovementRepository
}

func NewListMovements(repo repositories.MovementRepository) *ListMovements {
	return &ListMovements{repo: repo}
}

func (uc *ListMovements) Execute(ctx context.Context, input dto.ListMovementsInput) (dto.ListMovementsOutput, error) {
	if input.UserID == "" {
		return dto.ListMovementsOutput{}, apperrors.ErrInvalidInput
	}
	if input.Limit < 0 || input.Offset < 0 {
		return dto.ListMovementsOutput{}, apperrors.ErrInvalidInput
	}

	movements, err := uc.repo.ListByUser(ctx, input.UserID, input.Currency, input.Limit, input.Offset)
	if err != nil {
		return dto.ListMovementsOutput{}, err
	}

	var balance int64
	for _, m := range movements {
		balance += m.Amount
	}

	return dto.ListMovementsOutput{Movements: movements, Balance: balance}, nil
}
