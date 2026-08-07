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
	PaymentMethod string

	// CategoryID references the shared categories registry (BACK-14
	// follow-up) — nil means genuinely uncategorized. Unlike most other
	// *ID fields here, there's no separately-carried display name on
	// this entity: category is a shared, contributor-maintained thing,
	// not something this movement owns a copy of. A human-readable name
	// is resolved at the infrastructure boundary on read (see
	// dto.MovementDTO.Category), never stored or passed through the
	// domain layer.
	CategoryID *string

	// AvoidabilityOverridePercent (0-100, BACK-14) is this movement's own
	// ad-hoc avoidability, for a genuine one-off spend that doesn't
	// deserve its own category. Wins over the movement's category's
	// avoidability_percent when set — see application/usecases' effective-
	// avoidability resolution helper.
	AvoidabilityOverridePercent *int

	// AccountID links the movement to the account the money moved
	// in/out of (nil when the user didn't say). Local-only: it is not
	// part of what syncs to ledger-service.
	AccountID *string

	// TransferID links the two movement rows (debit + credit) that make
	// up one account-to-account transfer; nil for a plain movement.
	// Local-only, like AccountID: the two legs sync independently and net
	// to zero in ledger-service, which never learns they're linked.
	TransferID *string

	// PlanID links a movement to a savings Plan it funds (BACK-10) — nil
	// for an untagged movement or one funding nothing. Local-only, like
	// AccountID/TransferID: ledger-service never learns about plans.
	// Never set on a stress-test plan's movements, because a stress-test
	// plan never has any — it's a pure simulation over real cashflow
	// numbers, nothing is ever posted for it.
	PlanID *string

	// Set only when the movement is one installment of a credit-card
	// purchase that was split (installments > 1).
	CreditCardPurchaseID *string
	InstallmentNumber    *int // 1-based

	// RecurringRuleID links a movement the recurring generator created
	// (BACK-07) back to its rule, purely for provenance/UI display —
	// nothing about cancel/edit/sync treats it differently from a
	// manually-entered movement.
	RecurringRuleID *string

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
	// SyncStatusLocal marks a movement created while the user's
	// effective ledger sync was off (BACK-13's user_settings) — it was
	// never going to sync, so "pending" would misreport it as queued
	// work. Not permanent: re-enabling sync reclassifies a user's
	// "local" movements back to "pending" (see
	// UpdateUserSettingsUseCase / MovementRepository.MarkLocalPending)
	// so the accumulated backlog gets pushed like anything else.
	SyncStatusLocal SyncStatus = "local"
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
