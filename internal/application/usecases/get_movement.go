package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type GetMovementUseCase interface {
	Execute(ctx context.Context, id string) (*dto.MovementDTO, error)
}

type getMovementUseCase struct {
	repo repositories.MovementRepository
}

// NewGetMovement returns interface type for dependency injection.
func NewGetMovement(repo repositories.MovementRepository) GetMovementUseCase {
	return &getMovementUseCase{repo: repo}
}

func (uc *getMovementUseCase) Execute(ctx context.Context, id string) (*dto.MovementDTO, error) {
	if id == "" {
		return nil, apperrors.ErrInvalidInput
	}

	return uc.repo.GetByID(ctx, id)
}
