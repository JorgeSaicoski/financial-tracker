package usecases

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

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
	account *entities.Account,
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
