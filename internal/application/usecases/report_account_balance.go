package usecases

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type reportAccountBalanceUseCase struct {
	accounts  repositories.AccountRepository
	movements repositories.MovementRepository
}

// NewReportAccountBalance returns interface type for dependency injection.
func NewReportAccountBalance(accounts repositories.AccountRepository, movements repositories.MovementRepository) ReportAccountBalanceUseCase {
	return &reportAccountBalanceUseCase{accounts: accounts, movements: movements}
}

func (uc *reportAccountBalanceUseCase) Execute(ctx context.Context, userID, accountID string, balance int64, timestamp time.Time) (AccountView, error) {
	if userID == "" || accountID == "" {
		return AccountView{}, apperrors.ErrInvalidInput
	}

	account, err := uc.accounts.GetByID(ctx, accountID)
	if err != nil {
		return AccountView{}, err
	}
	if account.UserID != userID {
		return AccountView{}, apperrors.ErrNotFound
	}

	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	if _, err := uc.accounts.AddSnapshot(ctx, &dto.AccountSnapshotDTO{
		AccountID: account.ID,
		Balance:   balance,
		Timestamp: timestamp,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		return AccountView{}, err
	}

	return buildAccountView(ctx, uc.accounts, uc.movements, account)
}
