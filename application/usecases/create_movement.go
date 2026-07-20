package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	"github.com/JorgeSaicoski/financial-tracker/domain/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type CreateMovementUseCase interface {
	Execute(ctx context.Context, userID string, amount int64, currency string) (*entities.Movement, error)
}

type createMovementUseCase struct {
	repo repositories.MovementRepository
}

// NewCreateMovement returns interface type for dependency injection.
func NewCreateMovement(repo repositories.MovementRepository) CreateMovementUseCase {
	return &createMovementUseCase{repo: repo}
}

func (uc *createMovementUseCase) Execute(ctx context.Context, userID string, amount int64, currency string) (*entities.Movement, error) {
	if userID == "" || currency == "" || amount == 0 {
		return nil, apperrors.ErrInvalidInput
	}

	movement := &entities.Movement{
		UserID:   userID,
		Amount:   amount,
		Currency: currency,
	}

	return uc.repo.Create(ctx, movement)
}
