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
	// exist or isn't owned by userID. Never touches IsDefault — see
	// SetDefault.
	Update(ctx context.Context, userID, id, name string, avoidabilityPercent *int) error
	// HasDefault reports whether userID already has a category flagged
	// IsDefault — used by ensureDefaultCategory to seed one exactly once
	// per user, the same lazy, absence-safe pattern EnsureByName gives
	// the system categories.
	HasDefault(ctx context.Context, userID string) (bool, error)
	// SetDefault atomically clears IsDefault on whatever category
	// currently carries it for userID (if any) and sets it on id — the
	// partial unique index on (user_id) WHERE is_default backs this, so
	// even a racing pair of calls can't leave two categories flagged
	// default. apperrors.ErrNotFound if id doesn't exist or isn't owned
	// by userID.
	SetDefault(ctx context.Context, userID, id string) error
	// DeleteAndReassign atomically reassigns every movement and
	// credit-card purchase referencing categoryID to defaultCategoryID,
	// then deletes categoryID — the id/deleteCategoryUseCase's answer to
	// what "deleting one still referenced by movements" now means with a
	// real category_id foreign key (BACK-14 follow-up); the caller
	// (deleteCategoryUseCase) has already verified categoryID isn't
	// itself the default. apperrors.ErrNotFound if categoryID doesn't
	// exist or isn't owned by userID.
	DeleteAndReassign(ctx context.Context, userID, categoryID, defaultCategoryID string) error
}
