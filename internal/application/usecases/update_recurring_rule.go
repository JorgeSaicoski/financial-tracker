package usecases

import (
	"context"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type updateRecurringRuleUseCase struct {
	rules repositories.RecurringRuleRepository
}

// NewUpdateRecurringRule returns interface type for dependency injection.
func NewUpdateRecurringRule(rules repositories.RecurringRuleRepository) UpdateRecurringRuleUseCase {
	return &updateRecurringRuleUseCase{rules: rules}
}

func (uc *updateRecurringRuleUseCase) Execute(ctx context.Context, userID, id string, input UpdateRecurringRuleInput) (*dto.RecurringRuleDTO, error) {
	existing, err := uc.rules.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing.UserID != userID {
		// Don't distinguish "doesn't exist" from "exists but isn't
		// yours" — either way the caller gets a plain 404.
		return nil, apperrors.ErrNotFound
	}

	description := existing.Description
	if input.Description != nil {
		description = *input.Description
	}
	category := existing.Category
	if input.Category != nil {
		if !entities.Category(*input.Category).IsValid() {
			return nil, fmt.Errorf("%w: unknown category %q", apperrors.ErrInvalidInput, *input.Category)
		}
		category = *input.Category
	}
	paymentMethod := existing.PaymentMethod
	if input.PaymentMethod != nil {
		if !entities.PaymentMethod(*input.PaymentMethod).IsValid() {
			return nil, fmt.Errorf("%w: unknown payment method %q", apperrors.ErrInvalidInput, *input.PaymentMethod)
		}
		paymentMethod = *input.PaymentMethod
	}
	accountID := existing.AccountID
	if input.AccountID != nil {
		if *input.AccountID == "" {
			accountID = nil
		} else {
			accountID = input.AccountID
		}
	}
	if err := uc.rules.UpdateMetadata(ctx, id, description, category, paymentMethod, accountID); err != nil {
		return nil, err
	}

	amount := existing.Amount
	if input.Amount != nil {
		amount = *input.Amount
	}
	currency := existing.Currency
	if input.Currency != nil {
		currency = *input.Currency
	}
	if amount == 0 || currency == "" {
		return nil, apperrors.ErrInvalidInput
	}
	if err := uc.rules.UpdateFinancial(ctx, id, amount, currency); err != nil {
		return nil, err
	}

	dayOfMonth := existing.DayOfMonth
	if input.DayOfMonth != nil {
		if !entities.ValidDayOfMonth(*input.DayOfMonth) {
			return nil, fmt.Errorf(`%w: day_of_month must be "1"-"28" or "last"`, apperrors.ErrInvalidInput)
		}
		dayOfMonth = *input.DayOfMonth
	}
	endsAt := existing.EndsAt
	if input.EndsAt != nil {
		if !input.EndsAt.After(existing.StartsAt) {
			return nil, fmt.Errorf("%w: ends_at must be after starts_at", apperrors.ErrInvalidInput)
		}
		endsAt = input.EndsAt
	}
	if err := uc.rules.UpdateSchedule(ctx, id, dayOfMonth, endsAt); err != nil {
		return nil, err
	}

	active := existing.Active
	if input.Active != nil {
		active = *input.Active
	}
	if err := uc.rules.SetActive(ctx, id, active); err != nil {
		return nil, err
	}

	return uc.rules.GetByID(ctx, id)
}
