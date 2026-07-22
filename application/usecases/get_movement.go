package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type getMovementUseCase struct {
	repo repositories.MovementRepository
}

// NewGetMovement returns interface type for dependency injection.
func NewGetMovement(repo repositories.MovementRepository) GetMovementUseCase {
	return &getMovementUseCase{repo: repo}
}

func (uc *getMovementUseCase) Execute(ctx context.Context, id string) (*entities.Movement, error) {
	if id == "" {
		return nil, apperrors.ErrInvalidInput
	}

	return uc.repo.GetByID(ctx, id)
}
