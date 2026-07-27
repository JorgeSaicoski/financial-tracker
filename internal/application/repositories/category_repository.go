package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// CategoryRepository is the per-user, extendable registry of category
// names (BACK-14) — same shape as CurrencyRepository, but scoped per
// user and carrying an AvoidabilityPercent per row. "transfer" and
// "income" are system categories: nil AvoidabilityPercent, and they
// can't be renamed or deleted — enforced by the usecase layer, not here
// (this contract has no notion of "system", same reasoning
// MovementRepository has no notion of "reversal cannot be edited").
type CategoryRepository interface {
	// EnsureByName returns the user's existing category matching name
	// (case-insensitive), or creates one at avoidabilityPercent if none
	// exists yet — idempotent, same "adding an existing one is a no-op"
	// shape as CurrencyRepository.Add, but returning the row since
	// callers (implicit registration on movement creation) need its id.
	EnsureByName(ctx context.Context, userID, name string, avoidabilityPercent *int) (*dto.CategoryDTO, error)
	// GetByID returns a category owned by userID. apperrors.ErrNotFound
	// if it doesn't exist or isn't owned by userID — same "don't
	// distinguish doesn't-exist from isn't-yours" rule as GetMovementUseCase.
	GetByID(ctx context.Context, userID, id string) (*dto.CategoryDTO, error)
	// ListByUser returns every category row for the user, name ascending.
	ListByUser(ctx context.Context, userID string) ([]*dto.CategoryDTO, error)
	// Create inserts a brand-new category row, generating its ID. Callers
	// (the usecase layer) reject duplicate names and reserved system
	// names first — this method does not check either.
	Create(ctx context.Context, c *dto.CategoryDTO) (*dto.CategoryDTO, error)
	// Update overwrites name and avoidability_percent for a category
	// owned by userID with exactly the values given (nil means store
	// NULL, a legitimate state for system categories) — same
	// always-overwrite convention as MovementRepository.UpdateMetadata;
	// the usecase layer resolves a partial PATCH into the full merged
	// values before calling this. apperrors.ErrNotFound if it doesn't
	// exist or isn't owned by userID.
	Update(ctx context.Context, userID, id, name string, avoidabilityPercent *int) error
	// Delete removes a category owned by userID. apperrors.ErrNotFound if
	// it doesn't exist or isn't owned by userID. Deleting one still
	// referenced by movements is allowed — it's a label, not an FK.
	Delete(ctx context.Context, userID, id string) error
}
