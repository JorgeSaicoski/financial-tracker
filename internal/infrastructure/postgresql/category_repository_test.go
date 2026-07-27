package postgresql

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func dtoCategory(name string, avoidabilityPercent *int, contributorIDs ...string) *dto.CategoryDTO {
	return &dto.CategoryDTO{Name: name, AvoidabilityPercent: avoidabilityPercent, ContributorIDs: contributorIDs}
}

func TestCategoryCreateGetRoundtrip(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()
	avoidability := 80

	created, err := repo.Create(ctx, dtoCategory("restaurants", &avoidability, "u1"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "restaurants" || got.AvoidabilityPercent == nil || *got.AvoidabilityPercent != 80 {
		t.Errorf("got %+v", got)
	}
	if len(got.ContributorIDs) != 1 || got.ContributorIDs[0] != "u1" {
		t.Errorf("want u1 as sole contributor, got %+v", got.ContributorIDs)
	}
}

// TestCategoryCreateTwoUsersSameNameBothSucceed guards the BACK-14
// follow-up model directly: categories are global and not unique by
// name at all — two different users creating "restaurant" each get
// their own row.
func TestCategoryCreateTwoUsersSameNameBothSucceed(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()

	a, err := repo.Create(ctx, dtoCategory("restaurant", nil, "u1"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.Create(ctx, dtoCategory("restaurant", nil, "u2"))
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Errorf("want two different rows, got the same id %q twice", a.ID)
	}
}

func TestCategoryListAllReturnsEveryUsersCategories(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()

	if _, err := repo.Create(ctx, dtoCategory("food", nil, "u1")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, dtoCategory("transport", nil, "u2")); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, c := range got {
		names[c.Name] = true
	}
	// The migration also seeds "transfer"/"income"/"other" — ListAll is
	// unfiltered, so those are expected alongside the two just created.
	if !names["food"] || !names["transport"] {
		t.Errorf("want both users' categories visible, got %+v", got)
	}
}

func TestCategoryUpdateRenamesAndChangesAvoidability(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, dtoCategory("food", nil, "u1"))
	if err != nil {
		t.Fatal(err)
	}

	newAvoidability := 30
	if err := repo.Update(ctx, created.ID, "dining out", &newAvoidability); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, created.ID)
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

	err := repo.Update(ctx, "missing-id", "whatever", nil)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestCategoryIsContributor(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, dtoCategory("food", nil, "u1"))
	if err != nil {
		t.Fatal(err)
	}

	is, err := repo.IsContributor(ctx, "u1", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !is {
		t.Error("want u1 to be a contributor")
	}
	is, err = repo.IsContributor(ctx, "u2", created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if is {
		t.Error("want u2 to not be a contributor")
	}
}

func TestCategoryCountByContributor(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()

	if _, err := repo.Create(ctx, dtoCategory("a", nil, "u1")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, dtoCategory("b", nil, "u1")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, dtoCategory("c", nil, "u2")); err != nil {
		t.Fatal(err)
	}

	n, err := repo.CountByContributor(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("count = %d, want 2", n)
	}
}

func TestCategoryHideIsPerUserAndIdempotent(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, dtoCategory("food", nil, "u1"))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Hide(ctx, "u2", created.ID); err != nil {
		t.Fatal(err)
	}
	// Idempotent: hiding again must not error.
	if err := repo.Hide(ctx, "u2", created.ID); err != nil {
		t.Fatal(err)
	}
	// The row itself is untouched — hiding is not a delete.
	got, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Errorf("want the category to still exist after being hidden, got %v", err)
	}
	if got.Name != "food" {
		t.Errorf("got %+v", got)
	}
}

// TestCategoryHideAndReassignMovesOnlyCallersOwnRows exercises the actual
// reassignment: movements and credit-card purchases the caller owns move
// onto the default, but another user's rows referencing the same shared
// category are untouched — the whole point of scoping reassignment to
// the caller (BACK-14 follow-up: categories are shared, so "whose
// default" would otherwise be ambiguous).
func TestCategoryHideAndReassignMovesOnlyCallersOwnRows(t *testing.T) {
	db := openTestDB(t)
	categories := NewCategoryRepository(db)
	movements := NewMovementRepository(db)
	purchases := NewCreditCardPurchaseRepository(db)
	ctx := context.Background()
	now := nowTruncated()

	def, err := categories.Create(ctx, dtoCategory("other", nil, "u1"))
	if err != nil {
		t.Fatal(err)
	}
	shared, err := categories.Create(ctx, dtoCategory("dining", nil, "u1"))
	if err != nil {
		t.Fatal(err)
	}

	m1, err := movements.Create(ctx, &dto.MovementDTO{
		UserID: "u1", Amount: -500, Currency: "usd", CategoryID: &shared.ID, PaymentMethod: "other",
		Status: "active", SyncStatus: "pending", Timestamp: now, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	p1, _, err := purchases.CreateWithInstallments(ctx, &dto.CreditCardPurchaseDTO{
		UserID: "u1", CategoryID: &shared.ID, TotalAmount: -1000, Currency: "usd",
		InstallmentCount: 1, PurchaseDate: now, Status: "active", CreatedAt: now,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A different user also references the same shared category — must
	// stay untouched by u1's reassignment.
	m2, err := movements.Create(ctx, &dto.MovementDTO{
		UserID: "u2", Amount: -300, Currency: "usd", CategoryID: &shared.ID, PaymentMethod: "other",
		Status: "active", SyncStatus: "pending", Timestamp: now, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := categories.HideAndReassign(ctx, "u1", shared.ID, def.ID); err != nil {
		t.Fatal(err)
	}

	gotM1, err := movements.GetByID(ctx, m1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotM1.CategoryID == nil || *gotM1.CategoryID != def.ID {
		t.Errorf("u1's movement category_id = %v, want reassigned to %q", gotM1.CategoryID, def.ID)
	}
	gotP1, err := purchases.GetByID(ctx, p1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotP1.CategoryID == nil || *gotP1.CategoryID != def.ID {
		t.Errorf("u1's purchase category_id = %v, want reassigned to %q", gotP1.CategoryID, def.ID)
	}
	gotM2, err := movements.GetByID(ctx, m2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotM2.CategoryID == nil || *gotM2.CategoryID != shared.ID {
		t.Errorf("u2's movement must NOT be reassigned by u1's delete, got %v", gotM2.CategoryID)
	}

	// The shared category row itself still exists — u2 can still use it.
	if _, err := categories.GetByID(ctx, shared.ID); err != nil {
		t.Errorf("want the shared category to still exist, got %v", err)
	}
}
