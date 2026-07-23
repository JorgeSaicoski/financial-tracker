package entities

import "time"

// Movement is a single financial movement (income or expense) for a user.
// Amount is stored in the smallest currency unit and its sign carries the
// direction, so there is no separate "type" field to keep in sync.
//
// financial-tracker's local database is the source of truth; SyncStatus
// tracks whether the movement has also been recorded in ledger-service.
type Movement struct {
	ID            string
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	Category      Category
	PaymentMethod PaymentMethod

	// AccountID links the movement to the account the money moved
	// in/out of (nil when the user didn't say). Local-only: it is not
	// part of what syncs to ledger-service.
	AccountID *string

	// TransferID links the two movement rows (debit + credit) that make
	// up one account-to-account transfer; nil for a plain movement.
	// Local-only, like AccountID: the two legs sync independently and net
	// to zero in ledger-service, which never learns they're linked.
	TransferID *string

	// Set only when the movement is one installment of a credit-card
	// purchase that was split (installments > 1).
	CreditCardPurchaseID *string
	InstallmentNumber    *int // 1-based

	// Status is "voided" only for movements cancelled before they ever
	// reached ledger-service. A synced movement stays "active" forever
	// (ledger-service never deletes); its cancellation is expressed by a
	// reversal movement, linked through the two fields below.
	Status               MovementStatus
	CancelsMovementID    *string // set on a reversal, points at the original
	ReversedByMovementID *string // set on the original once a reversal exists

	Timestamp time.Time // effective date; future for not-yet-due installments

	SyncStatus          SyncStatus
	LedgerTransactionID *string
	SyncAttempts        int
	LastSyncError       *string
	LastSyncAttemptAt   *time.Time
	SyncedAt            *time.Time

	CreatedAt time.Time
}

type MovementStatus string

const (
	MovementStatusActive MovementStatus = "active"
	MovementStatusVoided MovementStatus = "voided"
)

type SyncStatus string

const (
	SyncStatusPending SyncStatus = "pending"
	SyncStatusSynced  SyncStatus = "synced"
	SyncStatusFailed  SyncStatus = "failed"
)

func (m Movement) IsCredit() bool {
	return m.Amount > 0
}

func (m Movement) IsDebit() bool {
	return m.Amount < 0
}

func (m Movement) IsSynced() bool {
	return m.SyncStatus == SyncStatusSynced
}

// IsReversal reports whether this movement exists to compensate another one.
func (m Movement) IsReversal() bool {
	return m.CancelsMovementID != nil
}

// IsCancelled covers both cancellation shapes: voided locally before
// syncing, or reversed by a compensating movement after syncing.
func (m Movement) IsCancelled() bool {
	return m.Status == MovementStatusVoided || m.ReversedByMovementID != nil
}
