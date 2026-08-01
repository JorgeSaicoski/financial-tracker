package usecases

import (
	"context"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type ensureUserUseCase struct {
	users    repositories.UserRepository
	settings repositories.UserSettingsRepository
}

// NewEnsureUser returns interface type for dependency injection.
func NewEnsureUser(users repositories.UserRepository, settings repositories.UserSettingsRepository) EnsureUserUseCase {
	return &ensureUserUseCase{users: users, settings: settings}
}

func (uc *ensureUserUseCase) Execute(ctx context.Context, input EnsureUserInput) (*dto.UserDTO, error) {
	if input.UserID == "" {
		return nil, fmt.Errorf("%w: user id is required", apperrors.ErrInvalidInput)
	}

	// Checked before Upsert, which would otherwise erase the very
	// distinction we need: "did a row already exist" is BACK-19's
	// grandfathering rule — a user who already existed before paid cloud
	// storage shipped must keep cloud_storage_entitled=true (the implicit
	// "no row" default), while a genuinely new signup gets an explicit
	// false row so it doesn't inherit that default by accident.
	existed, err := uc.users.Exists(ctx, input.UserID)
	if err != nil {
		return nil, err
	}

	user, err := uc.users.Upsert(ctx, &dto.UserDTO{
		ID:          input.UserID,
		Provider:    input.Provider,
		ExternalID:  input.ExternalID,
		Email:       input.Email,
		DisplayName: input.DisplayName,
	})
	if err != nil {
		return nil, err
	}

	if !existed {
		if _, err := uc.settings.SetCloudStorageEntitled(ctx, input.UserID, false); err != nil {
			return nil, err
		}
	}

	return user, nil
}
