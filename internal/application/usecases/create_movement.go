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
	methods  repositories.PaymentMethodRepository
	settings repositories.UserSettingsRepository
}

// NewCreateMovement returns interface type for dependency injection.
func NewCreateMovement(repo repositories.MovementRepository, accounts repositories.AccountRepository, methods repositories.PaymentMethodRepository, settings repositories.UserSettingsRepository) CreateMovementUseCase {
	return &createMovementUseCase{repo: repo, accounts: accounts, methods: methods, settings: settings}
}

func (uc *createMovementUseCase) Execute(ctx context.Context, input CreateMovementInput) (*dto.MovementDTO, error) {
	if input.UserID == "" || input.Currency == "" || input.Amount == 0 {
		return nil, apperrors.ErrInvalidInput
	}

	category, err := normalizeCategory(input.Category)
	if err != nil {
		return nil, err
	}
	paymentMethod, err := resolvePaymentMethod(ctx, uc.methods, input.UserID, input.PaymentMethod)
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
		Status:        entities.MovementStatusActive,
		SyncStatus:    syncStatus,
		Timestamp:     now,
		CreatedAt:     now,
	}

	return uc.repo.Create(ctx, dto.MovementFromEntity(movement))
}

// normalizeCategory applies the empty-means-other default and rejects
// values outside the domain's fixed category list. Payment method no
// longer goes through here (BACK-17 turned it into a per-user registry,
// resolved via resolvePaymentMethod in payment_methods.go).
func normalizeCategory(category string) (entities.Category, error) {
	c := entities.Category(category)
	if c == "" {
		c = entities.CategoryOther
	}
	if !c.IsValid() {
		return "", apperrors.ErrInvalidInput
	}
	return c, nil
}
