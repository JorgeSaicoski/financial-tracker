package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type getMovementUseCase struct {
	repo repositories.MovementRepository
}

// NewGetMovement returns interface type for dependency injection.
func NewGetMovement(repo repositories.MovementRepository) GetMovementUseCase {
	return &getMovementUseCase{repo: repo}
}

func (uc *getMovementUseCase) Execute(ctx context.Context, userID, id string) (*dto.MovementDTO, error) {
	if userID == "" || id == "" {
		return nil, apperrors.ErrInvalidInput
	}

	movement, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if movement.UserID != userID {
		// Don't distinguish "doesn't exist" from "exists but isn't
		// yours" — either way the caller gets a plain 404.
		return nil, apperrors.ErrNotFound
	}
	return movement, nil
}
