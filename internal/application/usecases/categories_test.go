package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func defaultLimits() *fakeLimitsRepo {
	return newFakeLimitsRepo(map[string]int{maxCategoriesPerUserLimit: 10})
}

func TestCreateCategoryRejectsReservedNames(t *testing.T) {
	categories := newFakeCategoryRepo()
	uc := NewCreateCategory(categories, defaultLimits())
	ctx := context.Background()

	for _, name := range []string{"transfer", "income", "other", "Transfer"} {
		if _, err := uc.Execute(ctx, CreateCategoryInput{UserID: "u1", Name: name}); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Errorf("name %q: want ErrInvalidInput, got %v", name, err)
		}
	}
}

func TestCreateCategoryTwoUsersSameNameGetDifferentIDs(t *testing.T) {
	categories := newFakeCategoryRepo()
	uc := NewCreateCategory(categories, defaultLimits())
	ctx := context.Background()

	a, err := uc.Execute(ctx, CreateCategoryInput{UserID: "u1", Name: "restaurant"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := uc.Execute(ctx, CreateCategoryInput{UserID: "u2", Name: "restaurant"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID {
		t.Errorf("want two different users' same-named categories to get different ids, both got %q", a.ID)
	}
	if len(a.ContributorIDs) != 1 || a.ContributorIDs[0] != "u1" {
		t.Errorf("want u1 sole contributor of a, got %+v", a.ContributorIDs)
	}
	if len(b.ContributorIDs) != 1 || b.ContributorIDs[0] != "u2" {
		t.Errorf("want u2 sole contributor of b, got %+v", b.ContributorIDs)
	}
}

func TestCreateCategoryEnforcesPerUserLimit(t *testing.T) {
	categories := newFakeCategoryRepo()
	limits := newFakeLimitsRepo(map[string]int{maxCategoriesPerUserLimit: 2})
	uc := NewCreateCategory(categories, limits)
	ctx := context.Background()

	if _, err := uc.Execute(ctx, CreateCategoryInput{UserID: "u1", Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Execute(ctx, CreateCategoryInput{UserID: "u1", Name: "b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Execute(ctx, CreateCategoryInput{UserID: "u1", Name: "c"}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput once the limit is reached, got %v", err)
	}
	// A different user is unaffected by u1's count.
	if _, err := uc.Execute(ctx, CreateCategoryInput{UserID: "u2", Name: "a"}); err != nil {
		t.Fatal(err)
	}
}

func TestListCategoriesReturnsEveryoneCategories(t *testing.T) {
	categories := newFakeCategoryRepo()
	create := NewCreateCategory(categories, defaultLimits())
	ctx := context.Background()

	if _, err := create.Execute(ctx, CreateCategoryInput{UserID: "u1", Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := create.Execute(ctx, CreateCategoryInput{UserID: "u2", Name: "b"}); err != nil {
		t.Fatal(err)
	}

	got, err := NewListCategories(categories).Execute(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want both users' categories visible to everyone, got %+v", got)
	}
}

func TestUpdateCategoryRejectsNonContributor(t *testing.T) {
	categories := newFakeCategoryRepo()
	create := NewCreateCategory(categories, defaultLimits())
	ctx := context.Background()

	created, err := create.Execute(ctx, CreateCategoryInput{UserID: "u1", Name: "groceries"})
	if err != nil {
		t.Fatal(err)
	}

	newName := "renamed"
	_, err = NewUpdateCategory(categories).Execute(ctx, "u2", created.ID, UpdateCategoryInput{Name: &newName})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for a non-contributor's edit, got %v", err)
	}
}

func TestUpdateCategoryAllowsContributor(t *testing.T) {
	categories := newFakeCategoryRepo()
	create := NewCreateCategory(categories, defaultLimits())
	ctx := context.Background()

	created, err := create.Execute(ctx, CreateCategoryInput{UserID: "u1", Name: "groceries"})
	if err != nil {
		t.Fatal(err)
	}

	newName := "supermarket"
	got, err := NewUpdateCategory(categories).Execute(ctx, "u1", created.ID, UpdateCategoryInput{Name: &newName})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "supermarket" {
		t.Errorf("want renamed category, got %+v", got)
	}
}

func TestUpdateCategoryRejectsSystemCategoryEdit(t *testing.T) {
	categories := newFakeCategoryRepo()
	ctx := context.Background()
	if _, err := categories.Create(ctx, &dto.CategoryDTO{ID: entities.CategoryOtherID, Name: entities.CategoryOther}); err != nil {
		t.Fatal(err)
	}

	newName := "renamed"
	_, err := NewUpdateCategory(categories).Execute(ctx, "u1", entities.CategoryOtherID, UpdateCategoryInput{Name: &newName})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput editing a system category, got %v", err)
	}
}

func TestDeleteCategoryHidesWithoutTouchingTheRow(t *testing.T) {
	categories := newFakeCategoryRepo()
	create := NewCreateCategory(categories, defaultLimits())
	ctx := context.Background()

	created, err := create.Execute(ctx, CreateCategoryInput{UserID: "u1", Name: "groceries"})
	if err != nil {
		t.Fatal(err)
	}

	if err := NewDeleteCategory(categories, newFakeUserSettingsRepo()).Execute(ctx, "u1", created.ID, false); err != nil {
		t.Fatal(err)
	}
	// The category row itself must still exist — hiding is per-user, not
	// a real delete (see the usecase's doc comment).
	if _, err := categories.GetByID(ctx, created.ID); err != nil {
		t.Errorf("want category to still exist after hide, got %v", err)
	}
	if !categories.hidden["u1"][created.ID] {
		t.Errorf("want the category marked hidden for u1")
	}
}

func TestDeleteCategoryRejectsSystemCategory(t *testing.T) {
	categories := newFakeCategoryRepo()
	ctx := context.Background()
	if _, err := categories.Create(ctx, &dto.CategoryDTO{ID: entities.CategoryTransferID, Name: entities.CategoryTransfer}); err != nil {
		t.Fatal(err)
	}

	err := NewDeleteCategory(categories, newFakeUserSettingsRepo()).Execute(ctx, "u1", entities.CategoryTransferID, false)
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput removing a system category, got %v", err)
	}
}

// TestTransferBetweenAccountsUsesFixedTransferCategoryID guards against a
// regression to the pre-BACK-14-follow-up behavior: Account.Send/Receive
// set CategoryID directly to the fixed, always-existing
// entities.CategoryTransferID — the usecase itself no longer touches the
// category repository at all (see NewTransferBetweenAccounts's shorter
// constructor).
func TestTransferBetweenAccountsUsesFixedTransferCategoryID(t *testing.T) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	ctx := context.Background()

	from, err := accounts.Create(ctx, &dto.AccountDTO{UserID: "u1", Currency: "usd"})
	if err != nil {
		t.Fatal(err)
	}
	to, err := accounts.Create(ctx, &dto.AccountDTO{UserID: "u1", Currency: "usd"})
	if err != nil {
		t.Fatal(err)
	}

	uc := NewTransferBetweenAccounts(movements, accounts, newFakePlanRepo(), newFakeUserSettingsRepo())
	result, err := uc.Execute(ctx, TransferBetweenAccountsInput{
		UserID: "u1", FromAccountID: from.ID, ToAccountID: to.ID, Amount: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, leg := range []*dto.MovementDTO{result.Debit, result.Credit} {
		if leg.CategoryID == nil || *leg.CategoryID != entities.CategoryTransferID {
			t.Errorf("want leg's category_id to be the fixed transfer category, got %+v", leg)
		}
	}
}

// TestImportArchiveReusesExistingCategoryIDAndCreatesMissingOnes guards
// the category_id-based restore contract (BACK-14 follow-up): a
// category_id already present in the target is reused untouched; one the
// target has never seen is created fresh, with the importer as its sole
// contributor and the archive's own denormalized name.
func TestImportArchiveReusesExistingCategoryIDAndCreatesMissingOnes(t *testing.T) {
	accounts := newFakeAccountRepo()
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)
	categories := newFakeCategoryRepo()
	ctx := context.Background()

	existing, err := categories.Create(ctx, &dto.CategoryDTO{Name: "hobbies", ContributorIDs: []string{"someone-else"}})
	if err != nil {
		t.Fatal(err)
	}

	uc := NewImportArchive(accounts, movements, purchases, categories)
	_, err = uc.Execute(ctx, "u1", ArchiveBundle{
		Movements: []*dto.MovementDTO{
			{ID: "m1", Amount: -100, Currency: "usd", CategoryID: &existing.ID, Category: "hobbies", PaymentMethod: "other",
				Status: "active", SyncStatus: "pending"},
		},
		CreditCardPurchases: []*dto.CreditCardPurchaseDTO{
			{ID: "p1", CategoryID: strPtrAv("new-electronics-id"), Category: "electronics", TotalAmount: -500, Currency: "usd",
				InstallmentCount: 1, Status: "active"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Reused: still owned by someone-else, u1 wasn't added as a contributor.
	reused, err := categories.GetByID(ctx, existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reused.ContributorIDs) != 1 || reused.ContributorIDs[0] != "someone-else" {
		t.Errorf("want the pre-existing category's contributors untouched, got %+v", reused.ContributorIDs)
	}

	// Created fresh: preserves the archive's id, u1 is sole contributor.
	created, err := categories.GetByID(ctx, "new-electronics-id")
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "electronics" {
		t.Errorf("want the archive's denormalized name, got %q", created.Name)
	}
	if len(created.ContributorIDs) != 1 || created.ContributorIDs[0] != "u1" {
		t.Errorf("want the importer as sole contributor of a newly created category, got %+v", created.ContributorIDs)
	}
}
