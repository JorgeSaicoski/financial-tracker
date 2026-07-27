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
	repo     repositories.MovementRepository
	accounts repositories.AccountRepository
	plans    repositories.PlanRepository
	settings repositories.UserSettingsRepository
}

// NewCreateMovement returns interface type for dependency injection.
func NewCreateMovement(repo repositories.MovementRepository, accounts repositories.AccountRepository, plans repositories.PlanRepository, settings repositories.UserSettingsRepository) CreateMovementUseCase {
	return &createMovementUseCase{repo: repo, accounts: accounts, plans: plans, settings: settings}
}

func (uc *createMovementUseCase) Execute(ctx context.Context, input CreateMovementInput) (*dto.MovementDTO, error) {
	if input.UserID == "" || input.Currency == "" || input.Amount == 0 {
		return nil, apperrors.ErrInvalidInput
	}

	category, paymentMethod, err := normalizeCategoryAndMethod(input.Category, input.PaymentMethod)
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
		UserID:        input.UserID,
		Amount:        input.Amount,
		Currency:      input.Currency,
		Description:   input.Description,
		Category:      category,
		PaymentMethod: paymentMethod,
		AccountID:     input.AccountID,
		PlanID:        planID,
		Status:        entities.MovementStatusActive,
		SyncStatus:    syncStatus,
		Timestamp:     now,
		CreatedAt:     now,
	}

	return uc.repo.Create(ctx, dto.MovementFromEntity(movement))
}

// normalizeCategoryAndMethod applies the empty-means-other default and
// rejects values outside the domain's fixed lists. Inputs arrive as
// plain strings (application/dto convention); the domain types do the
// validating.
func normalizeCategoryAndMethod(category, method string) (entities.Category, entities.PaymentMethod, error) {
	c := entities.Category(category)
	m := entities.PaymentMethod(method)
	if c == "" {
		c = entities.CategoryOther
	}
	if m == "" {
		m = entities.PaymentMethodOther
	}
	if !c.IsValid() || !m.IsValid() {
		return "", "", apperrors.ErrInvalidInput
	}
	return c, m, nil
}
