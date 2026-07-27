package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestListCategoriesSeedsDefaultCategoryAlongsideSystemOnes(t *testing.T) {
	categories := newFakeCategoryRepo()
	uc := NewListCategories(categories)
	ctx := context.Background()

	got, err := uc.Execute(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}

	var defaults int
	var sawOther bool
	for _, c := range got {
		if c.IsDefault {
			defaults++
		}
		if c.Name == defaultCategoryName {
			sawOther = true
			if c.IsDefault != true {
				t.Errorf("%q should be flagged default, got %+v", defaultCategoryName, c)
			}
		}
	}
	if defaults != 1 {
		t.Errorf("want exactly 1 default category, got %d: %+v", defaults, got)
	}
	if !sawOther {
		t.Errorf("want %q seeded as the default, got %+v", defaultCategoryName, got)
	}

	// Calling Execute again must not seed a second default.
	again, err := uc.Execute(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(got) {
		t.Errorf("second Execute call changed the category count: %dvs%d", len(got), len(again))
	}
}

func TestUpdateCategorySetsNewDefaultAndClearsOld(t *testing.T) {
	categories := newFakeCategoryRepo()
	ctx := context.Background()

	// Seed the default via ListCategories, same as a real request would.
	if _, err := NewListCategories(categories).Execute(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	created, err := NewCreateCategory(categories).Execute(ctx, CreateCategoryInput{UserID: "u1", Name: "groceries"})
	if err != nil {
		t.Fatal(err)
	}

	isDefault := true
	got, err := NewUpdateCategory(categories).Execute(ctx, "u1", created.ID, UpdateCategoryInput{IsDefault: &isDefault})
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsDefault {
		t.Errorf("want %q to become the default, got %+v", created.Name, got)
	}

	all, err := categories.ListByUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	var defaults int
	for _, c := range all {
		if c.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Errorf("want exactly 1 default after reassigning, got %d: %+v", defaults, all)
	}
}

func TestUpdateCategoryRejectsIsDefaultFalse(t *testing.T) {
	categories := newFakeCategoryRepo()
	ctx := context.Background()
	created, err := NewCreateCategory(categories).Execute(ctx, CreateCategoryInput{UserID: "u1", Name: "groceries"})
	if err != nil {
		t.Fatal(err)
	}

	isDefault := false
	_, err = NewUpdateCategory(categories).Execute(ctx, "u1", created.ID, UpdateCategoryInput{IsDefault: &isDefault})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for is_default:false, got %v", err)
	}
}

func TestDeleteCategoryRejectsCurrentDefault(t *testing.T) {
	categories := newFakeCategoryRepo()
	ctx := context.Background()

	// ensureDefaultCategory seeds and flags "other" as the default.
	if _, err := NewListCategories(categories).Execute(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	all, err := categories.ListByUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	var defaultID string
	for _, c := range all {
		if c.IsDefault {
			defaultID = c.ID
		}
	}
	if defaultID == "" {
		t.Fatal("no default category seeded")
	}

	err = NewDeleteCategory(categories).Execute(ctx, "u1", defaultID)
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput deleting the default category, got %v", err)
	}
}

func TestDeleteCategoryOfNonDefaultSucceeds(t *testing.T) {
	categories := newFakeCategoryRepo()
	ctx := context.Background()

	created, err := NewCreateCategory(categories).Execute(ctx, CreateCategoryInput{UserID: "u1", Name: "groceries"})
	if err != nil {
		t.Fatal(err)
	}

	if err := NewDeleteCategory(categories).Execute(ctx, "u1", created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := categories.GetByID(ctx, "u1", created.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
	// Deleting a non-default category with no prior GET /categories call
	// must still lazily seed a default to reassign onto, rather than
	// erroring because none exists yet.
	all, err := categories.ListByUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	var sawDefault bool
	for _, c := range all {
		if c.IsDefault {
			sawDefault = true
		}
	}
	if !sawDefault {
		t.Errorf("want a default category lazily seeded by delete, got %+v", all)
	}
}

// TestTransferBetweenAccountsEnsuresTransferCategoryExists guards the
// gap found while adding category_id as a real foreign key (BACK-14
// follow-up): Account.Send/Receive build a transfer leg with
// entities.CategoryTransfer directly, bypassing resolveCategory, so a
// brand-new user who transfers before ever calling GET /categories
// wouldn't have that category registered without this.
func TestTransferBetweenAccountsEnsuresTransferCategoryExists(t *testing.T) {
	movements := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	categories := newFakeCategoryRepo()
	ctx := context.Background()

	from, err := accounts.Create(ctx, &dto.AccountDTO{UserID: "u1", Currency: "usd"})
	if err != nil {
		t.Fatal(err)
	}
	to, err := accounts.Create(ctx, &dto.AccountDTO{UserID: "u1", Currency: "usd"})
	if err != nil {
		t.Fatal(err)
	}

	uc := NewTransferBetweenAccounts(movements, accounts, newFakeUserSettingsRepo(), categories)
	if _, err := uc.Execute(ctx, TransferBetweenAccountsInput{
		UserID: "u1", FromAccountID: from.ID, ToAccountID: to.ID, Amount: 1000,
	}); err != nil {
		t.Fatal(err)
	}

	all, err := categories.ListByUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	var sawTransfer bool
	for _, c := range all {
		if c.Name == entities.CategoryTransfer {
			sawTransfer = true
		}
	}
	if !sawTransfer {
		t.Errorf("want %q registered after a transfer, got %+v", entities.CategoryTransfer, all)
	}
}

// TestImportArchiveRegistersCategoriesFromBundle guards the other gap
// found alongside the transfer one: restore writes the archive's
// category names directly via CreateBatch/CreateWithInstallments,
// bypassing resolveCategory, so a category new to the target
// environment wouldn't be registered without this.
func TestImportArchiveRegistersCategoriesFromBundle(t *testing.T) {
	accounts := newFakeAccountRepo()
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)
	categories := newFakeCategoryRepo()
	ctx := context.Background()

	uc := NewImportArchive(accounts, movements, purchases, categories)
	_, err := uc.Execute(ctx, "u1", ArchiveBundle{
		Movements: []*dto.MovementDTO{
			{ID: "m1", Amount: -100, Currency: "usd", Category: "hobbies", PaymentMethod: "other",
				Status: "active", SyncStatus: "pending"},
		},
		CreditCardPurchases: []*dto.CreditCardPurchaseDTO{
			{ID: "p1", Category: "electronics", TotalAmount: -500, Currency: "usd",
				InstallmentCount: 1, Status: "active"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	all, err := categories.ListByUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, c := range all {
		names[c.Name] = true
	}
	if !names["hobbies"] || !names["electronics"] {
		t.Errorf("want both bundle categories registered, got %+v", all)
	}
}
