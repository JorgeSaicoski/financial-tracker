package dto

import "time"

// LocalArchiveSettingResponse is the body of GET/PUT /settings/local-archive.
type LocalArchiveSettingResponse struct {
	Enabled bool `json:"local_archive_enabled"`
}

// SetLocalArchiveSettingRequest is the body of PUT /settings/local-archive.
type SetLocalArchiveSettingRequest struct {
	Enabled bool `json:"local_archive_enabled"`
}

// ArchiveAccountDTO is an account's full restorable state — unlike
// AccountResponse, it carries no derived balance fields: the archive
// round-trips raw rows, it isn't a display view.
type ArchiveAccountDTO struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
}

// ArchiveMovementDTO mirrors application/dto.MovementDTO in full, including
// the sync-bookkeeping fields MovementResponse omits — a restore should
// reproduce the exact row the export saw, not just what the UI displays.
//
// cancels_movement_id/reversed_by_movement_id are included on export for
// completeness but are dropped by the import side (see the import usecase
// for why: a self-referencing foreign-key ordering problem neither SQLite
// nor Postgres defers here).
type ArchiveMovementDTO struct {
	ID            string `json:"id"`
	UserID        string `json:"user_id"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	Description   string `json:"description,omitempty"`
	Category      string `json:"category"`
	PaymentMethod string `json:"payment_method"`

	AccountID  string `json:"account_id,omitempty"`
	TransferID string `json:"transfer_id,omitempty"`

	CreditCardPurchaseID string `json:"credit_card_purchase_id,omitempty"`
	InstallmentNumber    int    `json:"installment_number,omitempty"`

	Status               string `json:"status"`
	CancelsMovementID    string `json:"cancels_movement_id,omitempty"`
	ReversedByMovementID string `json:"reversed_by_movement_id,omitempty"`

	Timestamp time.Time `json:"timestamp"`

	SyncStatus          string     `json:"sync_status"`
	LedgerTransactionID string     `json:"ledger_transaction_id,omitempty"`
	SyncAttempts        int        `json:"sync_attempts"`
	LastSyncError       string     `json:"last_sync_error,omitempty"`
	LastSyncAttemptAt   *time.Time `json:"last_sync_attempt_at,omitempty"`
	SyncedAt            *time.Time `json:"synced_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// ArchiveCreditCardPurchaseDTO mirrors application/dto.CreditCardPurchaseDTO.
type ArchiveCreditCardPurchaseDTO struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	Description      string    `json:"description,omitempty"`
	Category         string    `json:"category"`
	TotalAmount      int64     `json:"total_amount"`
	Currency         string    `json:"currency"`
	InstallmentCount int       `json:"installment_count"`
	PurchaseDate     time.Time `json:"purchase_date"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
}

// ArchiveResponse is the full account export (GET /export/archive) —
// everything needed to restore the account's state elsewhere. It doubles
// as ImportArchiveRequest's shape: posting an unmodified export back to
// POST /import/archive round-trips to the same state (BACK-15's
// acceptance criteria), modulo the cancel/reversal links noted above.
//
// Categories aren't included: they're still the fixed, hardcoded list
// (BACK-14 hasn't landed), so there's nothing user-defined to restore
// there yet.
type ArchiveResponse struct {
	ExportedAt          time.Time                      `json:"exported_at"`
	UserID              string                         `json:"user_id"`
	Accounts            []ArchiveAccountDTO            `json:"accounts"`
	Movements           []ArchiveMovementDTO           `json:"movements"`
	CreditCardPurchases []ArchiveCreditCardPurchaseDTO `json:"credit_card_purchases"`
}

// ImportArchiveRequest is the body of POST /import/archive — the same
// shape ArchiveResponse exports (minus the top-level UserID, which like
// every other write request always comes from the caller's verified
// token, never the body), so the frontend's decrypted archive can be
// posted back unmodified.
type ImportArchiveRequest struct {
	Accounts            []ArchiveAccountDTO            `json:"accounts"`
	Movements           []ArchiveMovementDTO           `json:"movements"`
	CreditCardPurchases []ArchiveCreditCardPurchaseDTO `json:"credit_card_purchases"`
}

// ImportArchiveResponse reports what the restore actually did.
type ImportArchiveResponse struct {
	AccountsRestored            int `json:"accounts_restored"`
	AccountsSkipped             int `json:"accounts_skipped"`
	MovementsRestored           int `json:"movements_restored"`
	MovementsSkipped            int `json:"movements_skipped"`
	CreditCardPurchasesRestored int `json:"credit_card_purchases_restored"`
	CreditCardPurchasesSkipped  int `json:"credit_card_purchases_skipped"`
}
