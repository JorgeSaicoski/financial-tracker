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

// defaultCategoryAvoidability is the neutral value a brand-new category
// gets when the caller doesn't specify one — the user edits it afterward.
const defaultCategoryAvoidability = 50

// maxCategoriesPerUserLimit is the name LimitsRepository looks this cap
// up under (BACK-14 follow-up: "I will not let him have 1000 categories
// and overload the CPU every single time I need to see his default" — an
// operator-configurable row, not a hardcoded constant, so retuning it is
// a database update, not a redeploy).
const maxCategoriesPerUserLimit = "max_categories_per_user"

// resolveCategoryID validates a caller-supplied category_id: nil or ""
// stays nil (genuinely uncategorized); anything else must reference an
// existing category — categories are globally visible (BACK-14
// follow-up), so no ownership check happens here, only existence.
func resolveCategoryID(ctx context.Context, categories repositories.CategoryRepository, categoryID *string) (*string, error) {
	if categoryID == nil || *categoryID == "" {
		return nil, nil
	}
	if _, err := categories.GetByID(ctx, *categoryID); err != nil {
		if apperrors.Is(err, apperrors.ErrNotFound) {
			return nil, fmt.Errorf("%w: category not found", apperrors.ErrInvalidInput)
		}
		return nil, err
	}
	return categoryID, nil
}

// resolveEffectiveDefaultCategoryID returns userID's own default category
// (user_settings.default_category_id), falling back to the global
// "other" row when they've never set one.
func resolveEffectiveDefaultCategoryID(ctx context.Context, settings repositories.UserSettingsRepository, userID string) (string, error) {
	s, err := settings.Get(ctx, userID)
	if err != nil {
		return "", err
	}
	if s.DefaultCategoryID != nil && *s.DefaultCategoryID != "" {
		return *s.DefaultCategoryID, nil
	}
	return entities.CategoryOtherID, nil
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
	limits     repositories.LimitsRepository
}

// NewCreateCategory returns interface type for dependency injection.
func NewCreateCategory(categories repositories.CategoryRepository, limits repositories.LimitsRepository) CreateCategoryUseCase {
	return &createCategoryUseCase{categories: categories, limits: limits}
}

func (uc *createCategoryUseCase) Execute(ctx context.Context, input CreateCategoryInput) (*dto.CategoryDTO, error) {
	name := strings.TrimSpace(input.Name)
	if input.UserID == "" || name == "" {
		return nil, fmt.Errorf("%w: category name is required", apperrors.ErrInvalidInput)
	}
	if entities.IsSystemCategoryName(name) {
		return nil, fmt.Errorf("%w: %q is a reserved category name", apperrors.ErrInvalidInput, name)
	}
	if err := validateAvoidabilityPercent(input.AvoidabilityPercent); err != nil {
		return nil, err
	}

	limit, err := uc.limits.GetValue(ctx, maxCategoriesPerUserLimit)
	if err != nil {
		return nil, err
	}
	count, err := uc.categories.CountByContributor(ctx, input.UserID)
	if err != nil {
		return nil, err
	}
	if count >= limit {
		return nil, fmt.Errorf("%w: you've reached the limit of %d categories — reuse an existing one instead of creating a new one",
			apperrors.ErrInvalidInput, limit)
	}

	avoidability := defaultCategoryAvoidability
	if input.AvoidabilityPercent != nil {
		avoidability = *input.AvoidabilityPercent
	}

	return uc.categories.Create(ctx, &dto.CategoryDTO{
		Name:                name,
		AvoidabilityPercent: &avoidability,
		ContributorIDs:      []string{input.UserID},
	})
}

type listCategoriesUseCase struct {
	categories repositories.CategoryRepository
}

// NewListCategories returns interface type for dependency injection.
func NewListCategories(categories repositories.CategoryRepository) ListCategoriesUseCase {
	return &listCategoriesUseCase{categories: categories}
}

func (uc *listCategoriesUseCase) Execute(ctx context.Context) ([]*dto.CategoryDTO, error) {
	return uc.categories.ListAll(ctx)
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
	existingDTO, err := uc.categories.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	category := categoryEntityFromDTO(existingDTO)

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: category name is required", apperrors.ErrInvalidInput)
		}
		if entities.IsSystemCategoryName(name) {
			return nil, fmt.Errorf("%w: %q is a reserved category name", apperrors.ErrInvalidInput, name)
		}
		if err := category.Rename(name, userID); err != nil {
			return nil, fmt.Errorf("%w: %v", apperrors.ErrInvalidInput, err)
		}
	}
	if input.AvoidabilityPercent != nil {
		if err := validateAvoidabilityPercent(input.AvoidabilityPercent); err != nil {
			return nil, err
		}
		if err := category.UpdateAvoidabilityPercent(input.AvoidabilityPercent, userID); err != nil {
			return nil, fmt.Errorf("%w: %v", apperrors.ErrInvalidInput, err)
		}
	}

	if err := uc.categories.Update(ctx, id, category.Name, category.AvoidabilityPercent); err != nil {
		return nil, err
	}
	return uc.categories.GetByID(ctx, id)
}

type deleteCategoryUseCase struct {
	categories repositories.CategoryRepository
	settings   repositories.UserSettingsRepository
}

// NewDeleteCategory returns interface type for dependency injection.
func NewDeleteCategory(categories repositories.CategoryRepository, settings repositories.UserSettingsRepository) DeleteCategoryUseCase {
	return &deleteCategoryUseCase{categories: categories, settings: settings}
}

func (uc *deleteCategoryUseCase) Execute(ctx context.Context, userID, id string, reassignExisting bool) error {
	if userID == "" || id == "" {
		return apperrors.ErrInvalidInput
	}
	existing, err := uc.categories.GetByID(ctx, id)
	if err != nil {
		return err
	}
	category := categoryEntityFromDTO(existing)
	if !category.CanBeHidden() {
		return fmt.Errorf("%w: %q is a reserved category and can't be removed", apperrors.ErrInvalidInput, category.Name)
	}

	if !reassignExisting {
		return uc.categories.Hide(ctx, userID, id)
	}

	defaultCategoryID, err := resolveEffectiveDefaultCategoryID(ctx, uc.settings, userID)
	if err != nil {
		return err
	}
	return uc.categories.HideAndReassign(ctx, userID, id, defaultCategoryID)
}

// categoryEntityFromDTO converts a CategoryDTO to the domain entity so
// the usecase can run its business-rule checks (CanBeEditedBy,
// CanBeHidden, Rename, UpdateAvoidabilityPercent) instead of duplicating
// them here.
func categoryEntityFromDTO(c *dto.CategoryDTO) *entities.Category {
	return &entities.Category{
		ID:                  c.ID,
		Name:                c.Name,
		AvoidabilityPercent: c.AvoidabilityPercent,
		ContributorIDs:      c.ContributorIDs,
		CreatedAt:           c.CreatedAt,
	}
}
