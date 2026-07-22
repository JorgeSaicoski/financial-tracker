package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
)

// AccountRepository persists accounts and their user-reported balance
// snapshots.
type AccountRepository interface {
	// Create inserts an account, generating its ID, and returns the
	// stored row.
	Create(ctx context.Context, account *entities.Account) (*entities.Account, error)
	GetByID(ctx context.Context, id string) (*entities.Account, error)
	ListByUser(ctx context.Context, userID string) ([]*entities.Account, error)

	// AddSnapshot inserts a reported-balance snapshot, generating its ID.
	AddSnapshot(ctx context.Context, snapshot *entities.AccountSnapshot) (*entities.AccountSnapshot, error)
	// LatestSnapshots returns up to n most recent snapshots for the
	// account, newest first.
	LatestSnapshots(ctx context.Context, accountID string, n int) ([]*entities.AccountSnapshot, error)
}
