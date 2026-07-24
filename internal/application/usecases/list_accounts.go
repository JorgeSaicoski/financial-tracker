package usecases

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// AccountView is an account plus everything derivable about its money:
//
//   - ReportedBalance/ReportedAt: the latest user-reported snapshot (nil
//     until the user reports one).
//   - MovementsSinceReport: net of tracked movements after that snapshot
//     (or all-time when there's no snapshot yet).
//   - EstimatedBalance: reported + movements since — our best guess of
//     what the account holds right now.
//   - LastReturn: between the last two snapshots, how much the balance
//     changed beyond what movements explain — the account's yield or
//     interest over LastReturnFrom..LastReturnTo. Needs two snapshots.
//
// Shared by ListAccountsUseCase (this file) and ReportAccountBalanceUseCase
// (report_account_balance.go) — both return the same shape via
// buildAccountView below.
type AccountView struct {
	Account              *dto.AccountDTO
	EstimatedBalance     int64
	ReportedBalance      *int64
	ReportedAt           *time.Time
	MovementsSinceReport int64
	LastReturn           *int64
	LastReturnFrom       *time.Time
	LastReturnTo         *time.Time
}

type ListAccountsUseCase interface {
	Execute(ctx context.Context, userID string) ([]AccountView, error)
}

type listAccountsUseCase struct {
	accounts  repositories.AccountRepository
	movements repositories.MovementRepository
}

// NewListAccounts returns interface type for dependency injection.
func NewListAccounts(accounts repositories.AccountRepository, movements repositories.MovementRepository) ListAccountsUseCase {
	return &listAccountsUseCase{accounts: accounts, movements: movements}
}

func (uc *listAccountsUseCase) Execute(ctx context.Context, userID string) ([]AccountView, error) {
	if userID == "" {
		return nil, apperrors.ErrInvalidInput
	}

	accounts, err := uc.accounts.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	views := make([]AccountView, 0, len(accounts))
	for _, account := range accounts {
		view, err := buildAccountView(ctx, uc.accounts, uc.movements, account)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// buildAccountView derives the balance picture for one account; shared
// with the snapshot-recording usecase so both return the same shape.
func buildAccountView(
	ctx context.Context,
	accounts repositories.AccountRepository,
	movements repositories.MovementRepository,
	account *dto.AccountDTO,
) (AccountView, error) {
	view := AccountView{Account: account}

	snapshots, err := accounts.LatestSnapshots(ctx, account.ID, 2)
	if err != nil {
		return AccountView{}, err
	}

	if len(snapshots) == 0 {
		// Nothing reported yet: the movements we tracked are all we know.
		net, err := movements.NetByAccount(ctx, account.ID, nil, nil)
		if err != nil {
			return AccountView{}, err
		}
		view.EstimatedBalance = net
		view.MovementsSinceReport = net
		return view, nil
	}

	latest := snapshots[0]
	view.ReportedBalance = &latest.Balance
	view.ReportedAt = &latest.Timestamp

	since, err := movements.NetByAccount(ctx, account.ID, &latest.Timestamp, nil)
	if err != nil {
		return AccountView{}, err
	}
	view.MovementsSinceReport = since
	view.EstimatedBalance = latest.Balance + since

	if len(snapshots) > 1 {
		previous := snapshots[1]
		between, err := movements.NetByAccount(ctx, account.ID, &previous.Timestamp, &latest.Timestamp)
		if err != nil {
			return AccountView{}, err
		}
		lastReturn := latest.Balance - previous.Balance - between
		view.LastReturn = &lastReturn
		view.LastReturnFrom = &previous.Timestamp
		view.LastReturnTo = &latest.Timestamp
	}
	return view, nil
}
