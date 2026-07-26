package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type getUserUseCase struct {
	users repositories.UserRepository
}

// NewGetUser returns interface type for dependency injection.
func NewGetUser(users repositories.UserRepository) GetUserUseCase {
	return &getUserUseCase{users: users}
}

func (uc *getUserUseCase) Execute(ctx context.Context, userID string) (*dto.UserDTO, error) {
	if userID == "" {
		return nil, apperrors.ErrInvalidInput
	}
	return uc.users.GetByID(ctx, userID)
}
