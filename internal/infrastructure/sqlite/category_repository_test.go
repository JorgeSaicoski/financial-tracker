package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
	if _, err := repo.Create(ctx, dtoCategory(userID, "Food", nil)); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("want ErrConflict inserting a case-variant duplicate name, got %v", err)
	}
}

// TestCategoryCreateConcurrentDuplicatesAllConflict exercises the explicit
// POST /categories path (unlike EnsureByName, a genuine duplicate here
// should fail, not silently resolve to the winner): several goroutines all
// call Create directly for the same new (user_id, lower(name)) — SQLite's
// single-connection pool serializes them, but the losers still hit the
// unique index and must get ErrConflict (409-mappable), not an
// unclassified error that writeUsecaseError would turn into a 500.
func TestCategoryCreateConcurrentDuplicatesAllConflict(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := repo.Create(ctx, dtoCategory(userID, "dining", nil))
			errs[i] = err
		}(i)
	}
	wg.Wait()

	var successes, conflicts int
	for i, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, apperrors.ErrConflict):
			conflicts++
		default:
			t.Errorf("goroutine %d: want nil or ErrConflict, got %v", i, err)
		}
	}
	if successes != 1 {
		t.Errorf("want exactly 1 successful create among %d concurrent calls, got %d", n, successes)
	}
	if conflicts != n-1 {
		t.Errorf("want %d ErrConflict losers, got %d", n-1, conflicts)
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

func TestCategoryDeleteAndReassignRemovesRow(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"

	def, err := repo.Create(ctx, dtoCategory(userID, "other", nil))
	if err != nil {
		t.Fatal(err)
	}
	created, err := repo.Create(ctx, dtoCategory(userID, "food", nil))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.DeleteAndReassign(ctx, userID, created.ID, def.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByID(ctx, userID, created.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

func TestCategoryDeleteAndReassignNotFound(t *testing.T) {
	repo := NewCategoryRepository(openTestDB(t))
	ctx := context.Background()

	def, err := repo.Create(ctx, dtoCategory("u1", "other", nil))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.DeleteAndReassign(ctx, "u1", "missing-id", def.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// TestCategoryDeleteAndReassignMovesMovementsAndPurchases exercises the
// actual reassignment: movements and credit-card purchases pointing at
// the deleted category must land on the default instead of getting
// orphaned — the whole reason DeleteAndReassign exists over a plain
// Delete now that category_id is a real foreign key (BACK-14 follow-up).
func TestCategoryDeleteAndReassignMovesMovementsAndPurchases(t *testing.T) {
	db := openTestDB(t)
	categories := NewCategoryRepository(db)
	movements := NewMovementRepository(db)
	purchases := NewCreditCardPurchaseRepository(db)
	ctx := context.Background()
	userID := "u1"
	now := time.Now().UTC()

	def, err := categories.Create(ctx, dtoCategory(userID, "other", nil))
	if err != nil {
		t.Fatal(err)
	}
	doomed, err := categories.Create(ctx, dtoCategory(userID, "dining", nil))
	if err != nil {
		t.Fatal(err)
	}

	m, err := movements.Create(ctx, &dto.MovementDTO{
		UserID: userID, Amount: -500, Currency: "usd", Category: "dining", PaymentMethod: "other",
		Status: "active", SyncStatus: "pending", Timestamp: now, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := purchases.CreateWithInstallments(ctx, &dto.CreditCardPurchaseDTO{
		UserID: userID, Category: "dining", TotalAmount: -1000, Currency: "usd",
		InstallmentCount: 1, PurchaseDate: now, Status: "active", CreatedAt: now,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := categories.DeleteAndReassign(ctx, userID, doomed.ID, def.ID); err != nil {
		t.Fatal(err)
	}

	gotMovement, err := movements.GetByID(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotMovement.Category != "other" {
		t.Errorf("movement category = %q, want reassigned to %q", gotMovement.Category, "other")
	}
	gotPurchase, err := purchases.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotPurchase.Category != "other" {
		t.Errorf("purchase category = %q, want reassigned to %q", gotPurchase.Category, "other")
	}
}
