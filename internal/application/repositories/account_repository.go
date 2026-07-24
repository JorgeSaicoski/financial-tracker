package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// AccountRepository persists accounts and their user-reported balance
// snapshots, expressed in application/dto types — infrastructure adapts
// its rows to these at the boundary.
type AccountRepository interface {
	// Create inserts an account, generating its ID, and returns the
	// stored row.
	Create(ctx context.Context, account *dto.AccountDTO) (*dto.AccountDTO, error)
	GetByID(ctx context.Context, id string) (*dto.AccountDTO, error)
	ListByUser(ctx context.Context, userID string) ([]*dto.AccountDTO, error)

	// AddSnapshot inserts a reported-balance snapshot, generating its ID.
	AddSnapshot(ctx context.Context, snapshot *dto.AccountSnapshotDTO) (*dto.AccountSnapshotDTO, error)
	// LatestSnapshots returns up to n most recent snapshots for the
	// account, newest first.
	LatestSnapshots(ctx context.Context, accountID string, n int) ([]*dto.AccountSnapshotDTO, error)
}
