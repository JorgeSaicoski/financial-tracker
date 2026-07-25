package usecases

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// maxSnapshotHistory bounds how many reported snapshots one account's
// history page shows. A personal-finance account reports a handful of
// times a year at most, so this is generous headroom, not a real limit
// — raise it (or make it a query param) if that assumption ever breaks.
const maxSnapshotHistory = 500

type listAccountSnapshotsUseCase struct {
	accounts  repositories.AccountRepository
	movements repositories.MovementRepository
}

// NewListAccountSnapshots returns interface type for dependency injection.
func NewListAccountSnapshots(accounts repositories.AccountRepository, movements repositories.MovementRepository) ListAccountSnapshotsUseCase {
	return &listAccountSnapshotsUseCase{accounts: accounts, movements: movements}
}

func (uc *listAccountSnapshotsUseCase) Execute(ctx context.Context, accountID string) ([]AccountSnapshotView, error) {
	if accountID == "" {
		return nil, apperrors.ErrInvalidInput
	}
	if _, err := uc.accounts.GetByID(ctx, accountID); err != nil {
		return nil, err
	}

	// LatestSnapshots returns newest-first; work oldest-first instead so
	// each entry's return pairs with the snapshot immediately before it,
	// then flip the result back to newest-first for display.
	snapshots, err := uc.accounts.LatestSnapshots(ctx, accountID, maxSnapshotHistory)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(snapshots)-1; i < j; i, j = i+1, j-1 {
		snapshots[i], snapshots[j] = snapshots[j], snapshots[i]
	}

	views := make([]AccountSnapshotView, len(snapshots))
	var previousTimestamp *time.Time
	var previousBalance int64
	for i, snap := range snapshots {
		net, err := uc.movements.NetByAccount(ctx, accountID, previousTimestamp, &snap.Timestamp)
		if err != nil {
			return nil, err
		}

		views[i] = AccountSnapshotView{
			Snapshot:   snap,
			Return:     snap.Balance - previousBalance - net,
			ReturnFrom: previousTimestamp,
			ReturnTo:   snap.Timestamp,
		}

		ts := snap.Timestamp
		previousTimestamp = &ts
		previousBalance = snap.Balance
	}

	for i, j := 0, len(views)-1; i < j; i, j = i+1, j-1 {
		views[i], views[j] = views[j], views[i]
	}
	return views, nil
}
