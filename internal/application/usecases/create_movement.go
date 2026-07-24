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

// CreateMovementInput carries the caller-supplied fields for a single
// movement. Category and PaymentMethod default to "other" when empty so
// pre-existing clients that only send an amount keep working; both are
// validated against the domain's fixed lists inside the usecase.
type CreateMovementInput struct {
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	Category      string
	PaymentMethod string
	AccountID     *string
}

type CreateMovementUseCase interface {
	Execute(ctx context.Context, input CreateMovementInput) (*dto.MovementDTO, error)
}

type createMovementUseCase struct {
	repo     repositories.MovementRepository
	accounts repositories.AccountRepository
}

// NewCreateMovement returns interface type for dependency injection.
func NewCreateMovement(repo repositories.MovementRepository, accounts repositories.AccountRepository) CreateMovementUseCase {
	return &createMovementUseCase{repo: repo, accounts: accounts}
}

func (uc *createMovementUseCase) Execute(ctx context.Context, input CreateMovementInput) (*dto.MovementDTO, error) {
	if input.UserID == "" || input.Currency == "" || input.Amount == 0 {
		return nil, apperrors.ErrInvalidInput
	}

	category, paymentMethod, err := normalizeCategoryAndMethod(input.Category, input.PaymentMethod)
	if err != nil {
		return nil, err
	}

	// An account holds one currency; a movement in a different currency
	// would silently corrupt that account's tracked balance.
	if input.AccountID != nil {
		account, err := uc.accounts.GetByID(ctx, *input.AccountID)
		if apperrors.Is(err, apperrors.ErrNotFound) {
			return nil, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
		}
		if err != nil {
			return nil, err
		}
		if account.Currency != input.Currency {
			return nil, fmt.Errorf("%w: movement currency %q does not match account currency %q",
				apperrors.ErrInvalidInput, input.Currency, account.Currency)
		}
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
		SyncStatus:    entities.SyncStatusPending,
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
