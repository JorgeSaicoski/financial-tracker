package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type GetMovement struct {
	repo repositories.MovementRepository
}

func NewGetMovement(repo repositories.MovementRepository) *GetMovement {
	return &GetMovement{repo: repo}
}

func (uc *GetMovement) Execute(ctx context.Context, id string) (dto.MovementOutput, error) {
	if id == "" {
		return dto.MovementOutput{}, apperrors.ErrInvalidInput
	}

	return uc.repo.GetByID(ctx, id)
}
