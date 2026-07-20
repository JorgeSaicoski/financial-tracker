package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type CreateMovement struct {
	repo repositories.MovementRepository
}

func NewCreateMovement(repo repositories.MovementRepository) *CreateMovement {
	return &CreateMovement{repo: repo}
}

func (uc *CreateMovement) Execute(ctx context.Context, input dto.CreateMovementInput) (dto.MovementOutput, error) {
	if input.UserID == "" || input.Currency == "" {
		return dto.MovementOutput{}, apperrors.ErrInvalidInput
	}
	if input.Amount == 0 {
		return dto.MovementOutput{}, apperrors.ErrInvalidInput
	}

	return uc.repo.Create(ctx, input)
}
