package usecases

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type reportAccountBalanceUseCase struct {
	accounts  repositories.AccountRepository
	movements repositories.MovementRepository
}

// NewReportAccountBalance returns interface type for dependency injection.
func NewReportAccountBalance(accounts repositories.AccountRepository, movements repositories.MovementRepository) ReportAccountBalanceUseCase {
	return &reportAccountBalanceUseCase{accounts: accounts, movements: movements}
}

func (uc *reportAccountBalanceUseCase) Execute(ctx context.Context, accountID string, balance int64) (AccountView, error) {
	if accountID == "" {
		return AccountView{}, apperrors.ErrInvalidInput
	}

	account, err := uc.accounts.GetByID(ctx, accountID)
	if err != nil {
		return AccountView{}, err
	}

	now := time.Now().UTC()
	if _, err := uc.accounts.AddSnapshot(ctx, &entities.AccountSnapshot{
		AccountID: account.ID,
		Balance:   balance,
		Timestamp: now,
		CreatedAt: now,
	}); err != nil {
		return AccountView{}, err
	}

	return buildAccountView(ctx, uc.accounts, uc.movements, account)
}
