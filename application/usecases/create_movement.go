package usecases

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	"github.com/JorgeSaicoski/financial-tracker/domain/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

// CreateMovementInput carries the caller-supplied fields for a single
// movement. Category and PaymentMethod default to "other" when empty so
// pre-existing clients that only send an amount keep working.
type CreateMovementInput struct {
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	Category      entities.Category
	PaymentMethod entities.PaymentMethod
}

type CreateMovementUseCase interface {
	Execute(ctx context.Context, input CreateMovementInput) (*entities.Movement, error)
}

type createMovementUseCase struct {
	repo repositories.MovementRepository
}

// NewCreateMovement returns interface type for dependency injection.
func NewCreateMovement(repo repositories.MovementRepository) CreateMovementUseCase {
	return &createMovementUseCase{repo: repo}
}

func (uc *createMovementUseCase) Execute(ctx context.Context, input CreateMovementInput) (*entities.Movement, error) {
	if input.UserID == "" || input.Currency == "" || input.Amount == 0 {
		return nil, apperrors.ErrInvalidInput
	}

	category, paymentMethod, err := normalizeCategoryAndMethod(input.Category, input.PaymentMethod)
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
		Status:        entities.MovementStatusActive,
		SyncStatus:    entities.SyncStatusPending,
		Timestamp:     now,
		CreatedAt:     now,
	}

	return uc.repo.Create(ctx, movement)
}

// normalizeCategoryAndMethod applies the empty-means-other default and
// rejects values outside the fixed lists.
func normalizeCategoryAndMethod(category entities.Category, method entities.PaymentMethod) (entities.Category, entities.PaymentMethod, error) {
	if category == "" {
		category = entities.CategoryOther
	}
	if method == "" {
		method = entities.PaymentMethodOther
	}
	if !category.IsValid() || !method.IsValid() {
		return "", "", apperrors.ErrInvalidInput
	}
	return category, method, nil
}
