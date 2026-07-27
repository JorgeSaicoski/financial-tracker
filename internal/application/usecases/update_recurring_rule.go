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
	rules      repositories.RecurringRuleRepository
	accounts   repositories.AccountRepository
	categories repositories.CategoryRepository
}

// NewUpdateRecurringRule returns interface type for dependency injection.
func NewUpdateRecurringRule(rules repositories.RecurringRuleRepository, accounts repositories.AccountRepository, categories repositories.CategoryRepository) UpdateRecurringRuleUseCase {
	return &updateRecurringRuleUseCase{rules: rules, accounts: accounts, categories: categories}
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

	// Every field is validated up front, before any repository write, so a
	// single PATCH either fully applies or fails clean — no partial update
	// left behind by a validation error discovered midway through.
	description := existing.Description
	if input.Description != nil {
		description = *input.Description
	}
	category := existing.Category
	if input.Category != nil {
		resolved, err := resolveCategory(ctx, uc.categories, userID, *input.Category)
		if err != nil {
			return nil, err
		}
		category = resolved
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

	// Same check CreateRecurringRule performs on the way in: a rule linked
	// to an account must generate movements in that account's currency, or
	// every future generation would fail CreateMovement's own currency
	// match check. Re-validated here (not just at create time) because
	// either accountID or currency can change independently via PATCH.
	if accountID != nil {
		account, err := uc.accounts.GetByID(ctx, *accountID)
		if apperrors.Is(err, apperrors.ErrNotFound) {
			return nil, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
		}
		if err != nil {
			return nil, err
		}
		if account.Currency != currency {
			return nil, fmt.Errorf("%w: rule currency %q does not match account currency %q",
				apperrors.ErrInvalidInput, currency, account.Currency)
		}
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
		// Inclusive cutoff, same as CreateRecurringRule: only a value
		// strictly before starts_at is rejected.
		if input.EndsAt.Before(existing.StartsAt) {
			return nil, fmt.Errorf("%w: ends_at must not be before starts_at", apperrors.ErrInvalidInput)
		}
		endsAt = input.EndsAt
	}

	active := existing.Active
	if input.Active != nil {
		active = *input.Active
	}

	if err := uc.rules.UpdateMetadata(ctx, id, description, category, paymentMethod, accountID); err != nil {
		return nil, err
	}
	if err := uc.rules.UpdateFinancial(ctx, id, amount, currency); err != nil {
		return nil, err
	}
	if err := uc.rules.UpdateSchedule(ctx, id, dayOfMonth, endsAt); err != nil {
		return nil, err
	}
	if err := uc.rules.SetActive(ctx, id, active); err != nil {
		return nil, err
	}

	return uc.rules.GetByID(ctx, id)
}
