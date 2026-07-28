package repositories

import (
	"context"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// CategoryRepository is the extendable registry of categories (BACK-14),
// shared across every user rather than scoped to one (BACK-14 follow-up,
// part 2: "I will create restaurant category with 80% and offer it for
// whoever wants to get it") — every category is globally visible and
// usable by anyone, referenced by id (there is no name-based resolution
// or lookup at all — category_id is required wherever a category is
// set, on a movement, purchase, or recurring rule). A CategoryDTO's
// ContributorIDs is who may edit it (rename, change
// avoidability_percent); it carries no notion of a single "owner" or
// "creator" — two different users independently creating "restaurant"
// each just get their own row, sole contributor themselves. "transfer",
// "income", and "other" are the three system-seeded categories (empty
// ContributorIDs, so no one can edit them): nil AvoidabilityPercent for
// transfer/income (not spend), and none of the three can be created,
// renamed, or hidden — enforced by the usecase layer via
// entities.Category, not here (this contract has no notion of
// "system", same reasoning MovementRepository has no notion of
// "reversal cannot be edited").
type CategoryRepository interface {
	// GetByID returns any category by id, with its current
	// ContributorIDs populated — categories are globally visible, there's
	// no ownership check here. apperrors.ErrNotFound if it doesn't exist.
	GetByID(ctx context.Context, id string) (*dto.CategoryDTO, error)
	// ListAll returns every category in the system, name ascending, each
	// with its ContributorIDs populated — backs GET /categories.
	ListAll(ctx context.Context) ([]*dto.CategoryDTO, error)
	// Create inserts a brand-new category row, generating its ID, and
	// atomically adds every id in c.ContributorIDs to
	// category_maintainers (in practice always exactly one — the
	// creator — since there's no add-contributor flow yet). Callers (the
	// usecase layer) reject reserved system names and check the
	// per-user category-count limit first — this method does not check
	// either.
	Create(ctx context.Context, c *dto.CategoryDTO) (*dto.CategoryDTO, error)
	// Update overwrites name and avoidability_percent for category id
	// with exactly the values given (nil means store NULL) — same
	// always-overwrite convention as MovementRepository.UpdateMetadata.
	// The usecase layer resolves a partial PATCH into the full merged
	// values and checks entities.Category.CanBeEditedBy before calling
	// this. apperrors.ErrNotFound if id doesn't exist.
	Update(ctx context.Context, id, name string, avoidabilityPercent *int) error
	// IsContributor reports whether userID may edit categoryID (rename,
	// change avoidability_percent). False, not apperrors.ErrNotFound,
	// when categoryID doesn't exist; callers that need existence
	// separately already called GetByID.
	IsContributor(ctx context.Context, userID, categoryID string) (bool, error)
	// CountByContributor returns how many categories userID currently
	// contributes to — backs createCategoryUseCase's per-user limit
	// check (see LimitsRepository, "max_categories_per_user").
	CountByContributor(ctx context.Context, userID string) (int, error)
	// Hide marks categoryID as opted out of userID's own future use —
	// idempotent, and never touches the category row itself or any
	// other user's data. This is what DELETE /categories/{id} means now
	// that a category may be shared: there is no real delete (see
	// deleteCategoryUseCase).
	Hide(ctx context.Context, userID, categoryID string) error
	// HideAndReassign does what Hide does, and — in the same
	// transaction — moves every movement and credit-card purchase userID
	// owns from categoryID onto defaultCategoryID first. Still scoped
	// strictly to userID's own rows, even though categoryID may be
	// referenced by other users too; their data is never touched.
	HideAndReassign(ctx context.Context, userID, categoryID, defaultCategoryID string) error
}
