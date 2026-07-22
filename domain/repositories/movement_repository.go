package repositories

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
)

// MovementRepository is the contract the domain depends on to persist and
// query movements. It is implemented by infrastructure/sqlite (the local
// source of truth) — usecases and handlers never know which backend is
// behind it.
type MovementRepository interface {
	// Create inserts a movement, generating its ID, and returns the
	// stored row.
	Create(ctx context.Context, movement *entities.Movement) (*entities.Movement, error)
	GetByID(ctx context.Context, id string) (*entities.Movement, error)
	ListByUser(ctx context.Context, userID string, currency *string, limit, offset int) ([]*entities.Movement, error)
	ListByCreditCardPurchase(ctx context.Context, purchaseID string) ([]*entities.Movement, error)

	// ListPendingSync returns active movements not yet synced to
	// ledger-service that are due (timestamp <= now) and were not
	// attempted within retryCooldown, oldest first. A zero cooldown
	// returns every due pending/failed row.
	ListPendingSync(ctx context.Context, now time.Time, retryCooldown time.Duration) ([]*entities.Movement, error)
	MarkSynced(ctx context.Context, id, ledgerTransactionID string, at time.Time) error
	MarkSyncFailed(ctx context.Context, id, syncErr string, at time.Time) error

	// Void marks a never-synced movement cancelled locally.
	Void(ctx context.Context, id string) error
	// CreateReversal atomically inserts the reversal (whose
	// CancelsMovementID must point at the original) and sets the
	// original's ReversedByMovementID. Returns ErrConflict if the
	// original is already reversed.
	CreateReversal(ctx context.Context, reversal *entities.Movement) (*entities.Movement, error)
}
