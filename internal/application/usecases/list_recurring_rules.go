package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type listRecurringRulesUseCase struct {
	rules repositories.RecurringRuleRepository
}

// NewListRecurringRules returns interface type for dependency injection.
func NewListRecurringRules(rules repositories.RecurringRuleRepository) ListRecurringRulesUseCase {
	return &listRecurringRulesUseCase{rules: rules}
}

func (uc *listRecurringRulesUseCase) Execute(ctx context.Context, userID string) ([]*dto.RecurringRuleDTO, error) {
	if userID == "" {
		return nil, apperrors.ErrInvalidInput
	}
	return uc.rules.ListByUser(ctx, userID)
}
