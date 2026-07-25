package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type getLocalArchiveSettingUseCase struct {
	repo repositories.LocalArchiveSettingsRepository
}

// NewGetLocalArchiveSetting returns interface type for dependency injection.
func NewGetLocalArchiveSetting(repo repositories.LocalArchiveSettingsRepository) GetLocalArchiveSettingUseCase {
	return &getLocalArchiveSettingUseCase{repo: repo}
}

func (uc *getLocalArchiveSettingUseCase) Execute(ctx context.Context, userID string) (bool, error) {
	if userID == "" {
		return false, apperrors.ErrInvalidInput
	}
	return uc.repo.IsEnabled(ctx, userID)
}

type setLocalArchiveSettingUseCase struct {
	repo repositories.LocalArchiveSettingsRepository
}

// NewSetLocalArchiveSetting returns interface type for dependency injection.
func NewSetLocalArchiveSetting(repo repositories.LocalArchiveSettingsRepository) SetLocalArchiveSettingUseCase {
	return &setLocalArchiveSettingUseCase{repo: repo}
}

func (uc *setLocalArchiveSettingUseCase) Execute(ctx context.Context, userID string, enabled bool) (bool, error) {
	if userID == "" {
		return false, apperrors.ErrInvalidInput
	}
	if err := uc.repo.SetEnabled(ctx, userID, enabled); err != nil {
		return false, err
	}
	return enabled, nil
}
