package usecases

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// ReportAccountBalanceUseCase records what the account really holds right
// now (per the bank/broker/wallet), as a snapshot. The returned view then
// exposes the account's return since the previous report. AccountView is
// defined in list_accounts.go (shared with ListAccountsUseCase).
type ReportAccountBalanceUseCase interface {
	Execute(ctx context.Context, accountID string, balance int64) (AccountView, error)
}

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
	if _, err := uc.accounts.AddSnapshot(ctx, &dto.AccountSnapshotDTO{
		AccountID: account.ID,
		Balance:   balance,
		Timestamp: now,
		CreatedAt: now,
	}); err != nil {
		return AccountView{}, err
	}

	return buildAccountView(ctx, uc.accounts, uc.movements, account)
}
