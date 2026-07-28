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

type createRecurringRuleUseCase struct {
	rules      repositories.RecurringRuleRepository
	accounts   repositories.AccountRepository
	methods    repositories.PaymentMethodRepository
	categories repositories.CategoryRepository
}

// NewCreateRecurringRule returns interface type for dependency injection.
func NewCreateRecurringRule(rules repositories.RecurringRuleRepository, accounts repositories.AccountRepository, methods repositories.PaymentMethodRepository, categories repositories.CategoryRepository) CreateRecurringRuleUseCase {
	return &createRecurringRuleUseCase{rules: rules, accounts: accounts, methods: methods, categories: categories}
}

func (uc *createRecurringRuleUseCase) Execute(ctx context.Context, input CreateRecurringRuleInput) (*dto.RecurringRuleDTO, error) {
	if input.UserID == "" || input.Currency == "" || input.Amount == 0 {
		return nil, apperrors.ErrInvalidInput
	}
	if !entities.ValidDayOfMonth(input.DayOfMonth) {
		return nil, fmt.Errorf(`%w: day_of_month must be "1"-"28" or "last"`, apperrors.ErrInvalidInput)
	}

	paymentMethod, err := resolvePaymentMethod(ctx, uc.methods, input.UserID, input.PaymentMethod)
	if err != nil {
		return nil, err
	}
	categoryID, err := resolveCategoryID(ctx, uc.categories, input.CategoryID)
	if err != nil {
		return nil, err
	}

	if input.AccountID != nil {
		account, err := uc.accounts.GetByID(ctx, *input.AccountID)
		if apperrors.Is(err, apperrors.ErrNotFound) {
			return nil, fmt.Errorf("%w: account not found", apperrors.ErrInvalidInput)
		}
		if err != nil {
			return nil, err
		}
		if account.Currency != input.Currency {
			return nil, fmt.Errorf("%w: rule currency %q does not match account currency %q",
				apperrors.ErrInvalidInput, input.Currency, account.Currency)
		}
	}

	startsAt := input.StartsAt
	if startsAt.IsZero() {
		startsAt = time.Now().UTC()
	}
	// Dates only, midnight UTC — the generator schedules by date, not
	// time of day, same normalization exchange rates use for
	// effective_from.
	startsAt = time.Date(startsAt.Year(), startsAt.Month(), startsAt.Day(), 0, 0, 0, 0, time.UTC)
	var endsAt *time.Time
	if input.EndsAt != nil {
		e := time.Date(input.EndsAt.Year(), input.EndsAt.Month(), input.EndsAt.Day(), 0, 0, 0, 0, time.UTC)
		endsAt = &e
	}

	// ends_at is an inclusive cutoff (the scheduler stops once an
	// occurrence is after ends_at), so a same-day rule (ends_at ==
	// starts_at) is valid — only a cutoff strictly before the start is
	// rejected. Validated after normalization so an omitted starts_at
	// (defaulted to today above) is checked against the real value, not
	// the zero time.
	if endsAt != nil && endsAt.Before(startsAt) {
		return nil, fmt.Errorf("%w: ends_at must not be before starts_at", apperrors.ErrInvalidInput)
	}

	rule := &entities.RecurringRule{
		UserID:        input.UserID,
		Amount:        input.Amount,
		Currency:      input.Currency,
		Description:   input.Description,
		CategoryID:    categoryID,
		PaymentMethod: paymentMethod,
		AccountID:     input.AccountID,
		DayOfMonth:    input.DayOfMonth,
		StartsAt:      startsAt,
		EndsAt:        endsAt,
		Active:        true,
		CreatedAt:     time.Now().UTC(),
	}

	return uc.rules.Create(ctx, dto.RecurringRuleFromEntity(rule))
}
