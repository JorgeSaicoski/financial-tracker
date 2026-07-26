package postgresql

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func dtoCategory(userID, name string, avoidabilityPercent *int) *dto.CategoryDTO {
	return &dto.CategoryDTO{UserID: userID, Name: name, AvoidabilityPercent: avoidabilityPercent}
}

func TestCategoryCreateGetRoundtrip(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"
	avoidability := 80

	created, err := repo.Create(ctx, dtoCategory(userID, "restaurants", &avoidability))
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, userID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "restaurants" || got.AvoidabilityPercent == nil || *got.AvoidabilityPercent != 80 {
		t.Errorf("got %+v", got)
	}
}

func TestCategoryEnsureByNameIsIdempotent(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"
	fifty := 50

	first, err := repo.EnsureByName(ctx, userID, "groceries", &fifty)
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.EnsureByName(ctx, userID, "Groceries", &fifty) // case-insensitive match
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Errorf("EnsureByName should return the existing row, got two different IDs: %s vs %s", first.ID, second.ID)
	}

	all, err := repo.ListByUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Errorf("want exactly one category after two EnsureByName calls for the same name, got %d", len(all))
	}
}

// TestCategoryCreateRejectsDuplicateName guards the unique (user_id,
// lower(name)) index the migration adds — the application layer's
// list-then-compare check is the common-path guard, but the DB
// constraint is the actual backstop against a race creating two rows
// for the same name.
func TestCategoryCreateRejectsDuplicateName(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"

	if _, err := repo.Create(ctx, dtoCategory(userID, "food", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, dtoCategory(userID, "Food", nil)); err == nil {
		t.Error("want an error inserting a case-variant duplicate name, got nil")
	}
}

func TestCategoryListByUserScopesToOwner(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()

	if _, err := repo.Create(ctx, dtoCategory("u1", "food", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, dtoCategory("u2", "transport", nil)); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListByUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "food" {
		t.Errorf("want exactly [food] for u1, got %+v", got)
	}
}

func TestCategoryUpdateRenamesAndChangesAvoidability(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"

	created, err := repo.Create(ctx, dtoCategory(userID, "food", nil))
	if err != nil {
		t.Fatal(err)
	}

	newAvoidability := 30
	if err := repo.Update(ctx, userID, created.ID, "dining out", &newAvoidability); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, userID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "dining out" || got.AvoidabilityPercent == nil || *got.AvoidabilityPercent != 30 {
		t.Errorf("got %+v", got)
	}
}

func TestCategoryUpdateNotFound(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()

	err := repo.Update(ctx, "u1", "missing-id", "whatever", nil)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestCategoryDeleteRemovesRow(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"

	created, err := repo.Create(ctx, dtoCategory(userID, "food", nil))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, userID, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByID(ctx, userID, created.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

func TestCategoryDeleteNotFound(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()

	err := repo.Delete(ctx, "u1", "missing-id")
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}
