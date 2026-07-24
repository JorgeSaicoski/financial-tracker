package repositories

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// MovementRepository is the contract the application depends on to
// persist and query movements, expressed in application/dto types —
// "application defines, infrastructure adapts". It is implemented by
// infrastructure/sqlite (the local source of truth), which converts its
// rows to dto.MovementDTO at this boundary — usecases and handlers never
// know which backend is behind it.
type MovementRepository interface {
	// Create inserts a movement, generating its ID, and returns the
	// stored row.
	Create(ctx context.Context, movement *dto.MovementDTO) (*dto.MovementDTO, error)
	// CreateBatch atomically inserts every movement, generating any
	// missing IDs — either all land or none do. Used for multi-leg
	// operations like transfers, where a partial insert would leave a
	// dangling, unbalanced leg.
	CreateBatch(ctx context.Context, movements []*dto.MovementDTO) ([]*dto.MovementDTO, error)
	GetByID(ctx context.Context, id string) (*dto.MovementDTO, error)
	// ListByUser filters by optional currency and optional [from, to)
	// time interval on the movement's effective timestamp.
	ListByUser(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) ([]*dto.MovementDTO, error)
	ListByCreditCardPurchase(ctx context.Context, purchaseID string) ([]*dto.MovementDTO, error)
	// ListByTransferID returns the legs (normally exactly two) sharing a
	// transfer_id, debit (negative amount) first.
	ListByTransferID(ctx context.Context, transferID string) ([]*dto.MovementDTO, error)

	// NetByAccount sums active movements of one account over (after,
	// until] — after exclusive so a snapshot taken at time T doesn't
	// double-count a movement stamped exactly T. Nil bounds mean open.
	NetByAccount(ctx context.Context, accountID string, after, until *time.Time) (int64, error)

	// ListPendingSync returns active movements not yet synced to
	// ledger-service that are due (timestamp <= now) and were not
	// attempted within retryCooldown, oldest first. A zero cooldown
	// returns every due pending/failed row.
	ListPendingSync(ctx context.Context, now time.Time, retryCooldown time.Duration) ([]*dto.MovementDTO, error)
	MarkSynced(ctx context.Context, id, ledgerTransactionID string, at time.Time) error
	MarkSyncFailed(ctx context.Context, id, syncErr string, at time.Time) error

	// UpdateMetadata overwrites the local-only fields — description,
	// category, payment method, account — regardless of sync status,
	// since none of them are ever pushed to ledger-service. Category and
	// paymentMethod arrive already validated by the usecase.
	UpdateMetadata(ctx context.Context, id, description, category, paymentMethod string, accountID *string) error
	// UpdateFinancial overwrites amount/currency/timestamp in place.
	// Callers must only use this on a movement that hasn't synced yet —
	// once ledger-service has it, these fields are immutable there.
	UpdateFinancial(ctx context.Context, id string, amount int64, currency string, timestamp time.Time) error

	// Void marks a never-synced movement cancelled locally.
	Void(ctx context.Context, id string) error
	// CreateReversal atomically inserts the reversal (whose
	// CancelsMovementID must point at the original) and sets the
	// original's ReversedByMovementID. Returns ErrConflict if the
	// original is already reversed.
	CreateReversal(ctx context.Context, reversal *dto.MovementDTO) (*dto.MovementDTO, error)

	// Transact executes fn inside a single database transaction. If fn
	// returns an error the transaction is rolled back; otherwise it is
	// committed. The MovementRepository passed to fn must only be used
	// within fn.
	Transact(ctx context.Context, fn func(tx MovementRepository) error) error
}
