package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

// effectiveSyncStatus is the sync_status a brand-new movement starts
// life with (BACK-13): "pending" when the user's effective ledger sync
// (entitled AND enabled) is on, "local" when it's off — a never-a-lie
// terminal state instead of a "pending" row that will never actually
// sync. Shared by every usecase that creates a movement from scratch
// (CreateMovement, CreateCreditCardPurchase, TransferBetweenAccounts).
// Usecases that create a *reversal* of an already-synced movement
// (CancelMovement, CancelCreditCardPurchase, CancelTransfer,
// UpdateMovement's reversal-then-replacement) deliberately don't use
// this: that reversal represents a real correction ledger-service needs
// eventually, so it stays "pending" regardless of the toggle's current
// state — re-enabling sync must not silently lose it.
func effectiveSyncStatus(ctx context.Context, settings repositories.UserSettingsRepository, userID string) (entities.SyncStatus, error) {
	s, err := settings.Get(ctx, userID)
	if err != nil {
		return "", err
	}
	if s.EffectiveLedgerSync() {
		return entities.SyncStatusPending, nil
	}
	return entities.SyncStatusLocal, nil
}

type getUserSettingsUseCase struct {
	repo repositories.UserSettingsRepository
}

// NewGetUserSettings returns interface type for dependency injection.
func NewGetUserSettings(repo repositories.UserSettingsRepository) GetUserSettingsUseCase {
	return &getUserSettingsUseCase{repo: repo}
}

func (uc *getUserSettingsUseCase) Execute(ctx context.Context, userID string) (UserSettingsView, error) {
	s, err := uc.repo.Get(ctx, userID)
	if err != nil {
		return UserSettingsView{}, err
	}
	return settingsViewFromDTO(s), nil
}

type updateUserSettingsUseCase struct {
	settings  repositories.UserSettingsRepository
	movements repositories.MovementRepository
}

// NewUpdateUserSettings returns interface type for dependency injection.
func NewUpdateUserSettings(settings repositories.UserSettingsRepository, movements repositories.MovementRepository) UpdateUserSettingsUseCase {
	return &updateUserSettingsUseCase{settings: settings, movements: movements}
}

func (uc *updateUserSettingsUseCase) Execute(ctx context.Context, userID string, ledgerSyncEnabled bool) (UserSettingsView, error) {
	before, err := uc.settings.Get(ctx, userID)
	if err != nil {
		return UserSettingsView{}, err
	}
	wasEffectivelyOn := before.EffectiveLedgerSync()

	after, err := uc.settings.UpdateEnabled(ctx, userID, ledgerSyncEnabled)
	if err != nil {
		return UserSettingsView{}, err
	}

	// Off -> on: the backlog created while sync was off is sitting as
	// "local" (see effectiveSyncStatus), never queried by the sync loop.
	// Reclassify it now so the very next pass pushes exactly that
	// backlog — BACK-13's acceptance criterion.
	if !wasEffectivelyOn && after.EffectiveLedgerSync() {
		if err := uc.movements.MarkLocalPending(ctx, userID); err != nil {
			return UserSettingsView{}, err
		}
	}

	return settingsViewFromDTO(after), nil
}

func settingsViewFromDTO(s *dto.UserSettingsDTO) UserSettingsView {
	return UserSettingsView{
		UserID:               s.UserID,
		LedgerSyncEntitled:   s.LedgerSyncEntitled,
		LedgerSyncEnabled:    s.LedgerSyncEnabled,
		CloudStorageEntitled: s.CloudStorageEntitled,
		CreatedAt:            s.CreatedAt,
		UpdatedAt:            s.UpdatedAt,
	}
}
