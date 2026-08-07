package usecases

import (
	"context"
	"fmt"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type createMovementUseCase struct {
	repo       repositories.MovementRepository
	accounts   repositories.AccountRepository
	methods    repositories.PaymentMethodRepository
	plans      repositories.PlanRepository
	categories repositories.CategoryRepository
	settings   repositories.UserSettingsRepository
}

// NewCreateMovement returns interface type for dependency injection.
func NewCreateMovement(repo repositories.MovementRepository, accounts repositories.AccountRepository, methods repositories.PaymentMethodRepository, plans repositories.PlanRepository, categories repositories.CategoryRepository, settings repositories.UserSettingsRepository) CreateMovementUseCase {
	return &createMovementUseCase{repo: repo, accounts: accounts, methods: methods, plans: plans, categories: categories, settings: settings}
}

func (uc *createMovementUseCase) Execute(ctx context.Context, input CreateMovementInput) (*dto.MovementDTO, error) {
	if input.UserID == "" || input.Currency == "" || input.Amount == 0 {
		return nil, apperrors.ErrInvalidInput
	}
	if err := validateAvoidabilityPercent(input.AvoidabilityOverridePercent); err != nil {
		return nil, err
	}

	paymentMethod, err := resolvePaymentMethod(ctx, uc.methods, input.UserID, input.PaymentMethod)
	if err != nil {
		return nil, err
	}
	categoryID, err := resolveCategoryID(ctx, uc.categories, input.UserID, input.CategoryID)
	if err != nil {
		return nil, err
	}

	var planIDInput string
	if input.PlanID != nil {
		planIDInput = *input.PlanID
	}
	planID, err := resolvePlanForMovement(ctx, uc.plans, input.UserID, planIDInput, input.Currency)
	if err != nil {
		return nil, err
	}

	// An account holds one currency; a movement in a different currency
	// would silently corrupt that account's tracked balance. Ownership is
	// checked here too (BACK-02) — without it, any authenticated user
	// could attach a movement to another user's account by guessing its
	// id, since currency-match alone doesn't prove ownership.
	if input.AccountID != nil {
		account, err := uc.accounts.GetByID(ctx, *input.AccountID)
		if apperrors.Is(err, apperrors.ErrNotFound) {
			return nil, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
		}
		if err != nil {
			return nil, err
		}
		if account.UserID != input.UserID {
			return nil, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
		}
		if account.Currency != input.Currency {
			return nil, fmt.Errorf("%w: movement currency %q does not match account currency %q",
				apperrors.ErrInvalidInput, input.Currency, account.Currency)
		}
	}

	syncStatus, err := effectiveSyncStatus(ctx, uc.settings, input.UserID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	movement := &entities.Movement{
		UserID:                      input.UserID,
		Amount:                      input.Amount,
		Currency:                    input.Currency,
		Description:                 input.Description,
		CategoryID:                  categoryID,
		PaymentMethod:               paymentMethod,
		AvoidabilityOverridePercent: input.AvoidabilityOverridePercent,
		AccountID:                   input.AccountID,
		PlanID:                      planID,
		Status:                      entities.MovementStatusActive,
		SyncStatus:                  syncStatus,
		Timestamp:                   now,
		CreatedAt:                   now,
	}

	return uc.repo.Create(ctx, dto.MovementFromEntity(movement))
}
