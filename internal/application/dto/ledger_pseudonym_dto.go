package dto

import "time"

// LedgerPseudonymDTO is BACK-16's per-user pseudonymous identity, sent to
// ledger-service in place of the real user id once ledger_sync_enabled is
// on. A random UUID, generated once and never derived from the real user
// id, so it can't be reversed. Minted lazily on first sync; never
// backfilled.
type LedgerPseudonymDTO struct {
	UserID      string
	PseudonymID string
	CreatedAt   time.Time
}
