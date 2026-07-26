package usecases

import (
	"context"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type ensureUserUseCase struct {
	users repositories.UserRepository
}

// NewEnsureUser returns interface type for dependency injection.
func NewEnsureUser(users repositories.UserRepository) EnsureUserUseCase {
	return &ensureUserUseCase{users: users}
}

func (uc *ensureUserUseCase) Execute(ctx context.Context, input EnsureUserInput) (*dto.UserDTO, error) {
	if input.UserID == "" {
		return nil, fmt.Errorf("%w: user id is required", apperrors.ErrInvalidInput)
	}

	return uc.users.Upsert(ctx, &dto.UserDTO{
		ID:          input.UserID,
		Provider:    input.Provider,
		ExternalID:  input.ExternalID,
		Email:       input.Email,
		DisplayName: input.DisplayName,
	})
}
