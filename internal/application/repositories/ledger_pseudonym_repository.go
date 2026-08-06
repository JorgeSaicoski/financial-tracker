package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// LedgerPseudonymRepository persists BACK-16's per-user pseudonymous
// ledger-service identity: a random UUID, generated once and never
// derived from the real user id, so it can't be reversed.
type LedgerPseudonymRepository interface {
	// Get returns userID's pseudonym row, or apperrors.ErrNotFound if
	// none exists yet.
	Get(ctx context.Context, userID string) (*dto.LedgerPseudonymDTO, error)
	// Create is race-safe the same way UserDataKeyRepository.Create is —
	// first writer wins, exactly one pseudonym per user, reused on every
	// subsequent sync (BACK-16 acceptance criterion).
	Create(ctx context.Context, row *dto.LedgerPseudonymDTO) (*dto.LedgerPseudonymDTO, error)
}
