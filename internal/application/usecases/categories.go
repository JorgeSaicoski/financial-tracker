package usecases

import (
	"context"
	"fmt"
	"strings"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

// defaultCategoryAvoidability is the neutral value a brand-new,
// implicitly-registered category gets (the user edits it afterward) —
// same default the one-time migration backfill uses.
const defaultCategoryAvoidability = 50

// defaultCategoryName is the fallback category lazily seeded per user
// (BACK-14 follow-up) and flagged IsDefault — the user is free to rename
// it or flag a different category as the default instead; "other" is
// just what a brand-new user starts with, not a reserved name like
// entities.CategoryTransfer/CategoryIncome.
const defaultCategoryName = "other"

// isSystemCategory reports whether name (case-insensitive) is one of the
// two reserved category names code branches on directly (Account.Send/
// Receive, getCashflowUseCase) — not spend, so they carry no
// avoidability and can't be created/renamed/deleted through the API.
func isSystemCategory(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return lower == entities.CategoryTransfer || lower == entities.CategoryIncome
}

// resolveCategory returns the effective category name to store on a
// movement: empty input stays empty (genuinely uncategorized — pairs
// with the movement's own avoidability override), otherwise the name is
// resolved against the user's category registry, implicitly registering
// it at a neutral 50% default on first use (same idempotent-Add shape as
// AddCurrencyUseCase) if it doesn't already exist. The two system names
// resolve to their (lazily-ensured) NULL-avoidability row instead of
// creating a duplicate 50%-avoidability one.
func resolveCategory(ctx context.Context, categories repositories.CategoryRepository, userID, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	var avoidabilityPercent *int
	if !isSystemCategory(name) {
		v := defaultCategoryAvoidability
		avoidabilityPercent = &v
	}
	if _, err := categories.EnsureByName(ctx, userID, name, avoidabilityPercent); err != nil {
		return "", err
	}
	return name, nil
}

// ensureSystemCategories lazily creates "transfer" and "income" for
// userID if they don't exist yet — absence-safe, same pattern as
// user_settings. Idempotent: EnsureByName no-ops when the row already
// exists.
func ensureSystemCategories(ctx context.Context, categories repositories.CategoryRepository, userID string) error {
	if _, err := categories.EnsureByName(ctx, userID, entities.CategoryTransfer, nil); err != nil {
		return err
	}
	if _, err := categories.EnsureByName(ctx, userID, entities.CategoryIncome, nil); err != nil {
		return err
	}
	return nil
}

// ensureDefaultCategory lazily seeds userID's fallback category
// (BACK-14 follow-up) the first time it's needed — same absence-safe
// pattern as ensureSystemCategories, but only acts once: if the user
// already has a category flagged IsDefault (seeded here earlier, or set
// explicitly since via updateCategoryUseCase), it's left alone. Returns
// the user's current default row either way, since deleteCategoryUseCase
// needs its id to reassign onto.
func ensureDefaultCategory(ctx context.Context, categories repositories.CategoryRepository, userID string) (*dto.CategoryDTO, error) {
	has, err := categories.HasDefault(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !has {
		avoidability := defaultCategoryAvoidability
		def, err := categories.EnsureByName(ctx, userID, defaultCategoryName, &avoidability)
		if err != nil {
			return nil, err
		}
		// A concurrent caller may have set a default between the
		// HasDefault check above and here (e.g. an explicit PATCH
		// is_default:true racing this lazy seed) — SetDefault would
		// just move it back, so re-check rather than blindly setting.
		has, err = categories.HasDefault(ctx, userID)
		if err != nil {
			return nil, err
		}
		if !has {
			if err := categories.SetDefault(ctx, userID, def.ID); err != nil {
				return nil, err
			}
		}
	}
	return getDefaultCategory(ctx, categories, userID)
}

// getDefaultCategory finds userID's current default row. Only called
// after ensureDefaultCategory has guaranteed one exists, so ErrNotFound
// here would mean that invariant broke rather than a normal "not found".
func getDefaultCategory(ctx context.Context, categories repositories.CategoryRepository, userID string) (*dto.CategoryDTO, error) {
	all, err := categories.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, c := range all {
		if c.IsDefault {
			return c, nil
		}
	}
	return nil, fmt.Errorf("%w: user has no default category", apperrors.ErrNotFound)
}

// validateAvoidabilityPercent rejects a value outside 0-100 — shared by
// category create/update and, via the movement usecases, the movement
// override field.
func validateAvoidabilityPercent(v *int) error {
	if v != nil && (*v < 0 || *v > 100) {
		return fmt.Errorf("%w: avoidability_percent must be 0-100", apperrors.ErrInvalidInput)
	}
	return nil
}

type createCategoryUseCase struct {
	categories repositories.CategoryRepository
}

// NewCreateCategory returns interface type for dependency injection.
func NewCreateCategory(categories repositories.CategoryRepository) CreateCategoryUseCase {
	return &createCategoryUseCase{categories: categories}
}

func (uc *createCategoryUseCase) Execute(ctx context.Context, input CreateCategoryInput) (*dto.CategoryDTO, error) {
	name := strings.TrimSpace(input.Name)
	if input.UserID == "" || name == "" {
		return nil, fmt.Errorf("%w: category name is required", apperrors.ErrInvalidInput)
	}
	if isSystemCategory(name) {
		return nil, fmt.Errorf("%w: %q is a reserved category name", apperrors.ErrInvalidInput, name)
	}
	if err := validateAvoidabilityPercent(input.AvoidabilityPercent); err != nil {
		return nil, err
	}

	avoidability := defaultCategoryAvoidability
	if input.AvoidabilityPercent != nil {
		avoidability = *input.AvoidabilityPercent
	}

	existing, err := uc.categories.ListByUser(ctx, input.UserID)
	if err != nil {
		return nil, err
	}
	for _, c := range existing {
		if strings.EqualFold(c.Name, name) {
			return nil, fmt.Errorf("%w: category %q already exists", apperrors.ErrConflict, name)
		}
	}

	return uc.categories.Create(ctx, &dto.CategoryDTO{
		UserID:              input.UserID,
		Name:                name,
		AvoidabilityPercent: &avoidability,
	})
}

type listCategoriesUseCase struct {
	categories repositories.CategoryRepository
}

// NewListCategories returns interface type for dependency injection.
func NewListCategories(categories repositories.CategoryRepository) ListCategoriesUseCase {
	return &listCategoriesUseCase{categories: categories}
}

func (uc *listCategoriesUseCase) Execute(ctx context.Context, userID string) ([]*dto.CategoryDTO, error) {
	if err := ensureSystemCategories(ctx, uc.categories, userID); err != nil {
		return nil, err
	}
	if _, err := ensureDefaultCategory(ctx, uc.categories, userID); err != nil {
		return nil, err
	}
	return uc.categories.ListByUser(ctx, userID)
}

type updateCategoryUseCase struct {
	categories repositories.CategoryRepository
}

// NewUpdateCategory returns interface type for dependency injection.
func NewUpdateCategory(categories repositories.CategoryRepository) UpdateCategoryUseCase {
	return &updateCategoryUseCase{categories: categories}
}

func (uc *updateCategoryUseCase) Execute(ctx context.Context, userID, id string, input UpdateCategoryInput) (*dto.CategoryDTO, error) {
	if userID == "" || id == "" {
		return nil, apperrors.ErrInvalidInput
	}
	existing, err := uc.categories.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if isSystemCategory(existing.Name) {
		return nil, fmt.Errorf("%w: %q is a reserved category and can't be edited", apperrors.ErrInvalidInput, existing.Name)
	}

	name := existing.Name
	if input.Name != nil {
		name = strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: category name is required", apperrors.ErrInvalidInput)
		}
		if isSystemCategory(name) {
			return nil, fmt.Errorf("%w: %q is a reserved category name", apperrors.ErrInvalidInput, name)
		}
	}
	if err := validateAvoidabilityPercent(input.AvoidabilityPercent); err != nil {
		return nil, err
	}
	avoidabilityPercent := existing.AvoidabilityPercent
	if input.AvoidabilityPercent != nil {
		avoidabilityPercent = input.AvoidabilityPercent
	}
	// IsDefault is a one-way flag here: setting it true makes id the
	// user's new default (atomically clearing whoever held it before —
	// see CategoryRepository.SetDefault); false is rejected rather than
	// silently ignored, since "no default" isn't a state this app
	// supports — the invariant is always exactly one, enforced by the
	// migration's partial unique index too.
	if input.IsDefault != nil && !*input.IsDefault {
		return nil, fmt.Errorf("%w: set is_default on the category you want as default instead of clearing it here", apperrors.ErrInvalidInput)
	}

	if !strings.EqualFold(name, existing.Name) {
		siblings, err := uc.categories.ListByUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, c := range siblings {
			if c.ID != id && strings.EqualFold(c.Name, name) {
				return nil, fmt.Errorf("%w: category %q already exists", apperrors.ErrConflict, name)
			}
		}
	}

	if err := uc.categories.Update(ctx, userID, id, name, avoidabilityPercent); err != nil {
		return nil, err
	}
	if input.IsDefault != nil && *input.IsDefault {
		if err := uc.categories.SetDefault(ctx, userID, id); err != nil {
			return nil, err
		}
	}
	return uc.categories.GetByID(ctx, userID, id)
}

type deleteCategoryUseCase struct {
	categories repositories.CategoryRepository
}

// NewDeleteCategory returns interface type for dependency injection.
func NewDeleteCategory(categories repositories.CategoryRepository) DeleteCategoryUseCase {
	return &deleteCategoryUseCase{categories: categories}
}

func (uc *deleteCategoryUseCase) Execute(ctx context.Context, userID, id string) error {
	if userID == "" || id == "" {
		return apperrors.ErrInvalidInput
	}
	existing, err := uc.categories.GetByID(ctx, userID, id)
	if err != nil {
		return err
	}
	if isSystemCategory(existing.Name) {
		return fmt.Errorf("%w: %q is a reserved category and can't be deleted", apperrors.ErrInvalidInput, existing.Name)
	}
	if existing.IsDefault {
		return fmt.Errorf("%w: %q is the default category and can't be deleted — set a different category as default first", apperrors.ErrInvalidInput, existing.Name)
	}
	// Every movement/purchase currently pointing at id needs somewhere to
	// land before the FK it holds disappears — ensureDefaultCategory
	// guarantees that target exists (lazily seeding "other" for a user
	// who has never triggered GET /categories) rather than assuming
	// listCategoriesUseCase already ran first.
	def, err := ensureDefaultCategory(ctx, uc.categories, userID)
	if err != nil {
		return err
	}
	return uc.categories.DeleteAndReassign(ctx, userID, id, def.ID)
}
