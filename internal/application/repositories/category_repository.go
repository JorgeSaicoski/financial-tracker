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
// renamed, or removed from a user's list — enforced by the usecase layer via
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
	// ListForUser returns the categories userID currently *has*, backed
	// by user_categories — a plain positive membership table, not an
	// opt-out/"hidden" one (Jorge, 2026-07-28: "there is no hidden
	// category"). The creator of a category always has it (contributor
	// and "has" are granted together at creation time, in the same
	// transaction as Create); it comes out of this list the moment
	// userID removes it (see Remove/RemoveAndReassign), even though they
	// may still be a contributor. Deliberately separate from "how many
	// is userID a contributor of" (Jorge, review comment on #39: "we
	// don't care how much categories he is contributor or not, we care
	// how much does he has — maybe he is contributor on 15 categories
	// but just have 10"). The usecase layer loads this into
	// entities.User.Categories and calls AddCategory/RemoveCategory on
	// it, rather than checking a count directly here.
	ListForUser(ctx context.Context, userID string) ([]*dto.CategoryDTO, error)
	// HasForUser reports whether userID currently has categoryID in
	// their own list (user_categories) — the enforcement half of
	// ListForUser: every write path that accepts a caller-supplied
	// category_id (movements, purchases, recurring rules, a user's own
	// default) must check this before accepting it, so a category
	// removed from a user's list can't be re-selected going forward.
	// Always true for a system category (transfer/income/other) even
	// though no one's user_categories row is ever seeded for them — see
	// resolveCategoryID, which is where that exception is applied.
	HasForUser(ctx context.Context, userID, categoryID string) (bool, error)
	// Remove takes categoryID out of userID's own list (deletes the
	// user_categories row) — never touches the category row itself,
	// category_maintainers, or any other user's data. This is what
	// DELETE /categories/{id} means now that a category may be shared:
	// there is no real delete (see deleteCategoryUseCase). Existing
	// movements/purchases/recurring rules already referencing categoryID
	// keep doing so unless the caller separately chose reassignment (see
	// RemoveAndReassign) — they just can't be created or re-pointed at
	// it going forward, per HasForUser.
	Remove(ctx context.Context, userID, categoryID string) error
	// RemoveAndReassign does what Remove does, and — in the same
	// transaction — moves every movement, credit-card purchase, and
	// recurring rule userID owns from categoryID onto defaultCategoryID
	// first. Still scoped strictly to userID's own rows, even though
	// categoryID may be referenced by other users too; their data is
	// never touched.
	RemoveAndReassign(ctx context.Context, userID, categoryID, defaultCategoryID string) error
}
