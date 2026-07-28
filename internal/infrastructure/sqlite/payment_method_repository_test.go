package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func dtoPaymentMethod(userID, name string) *dto.PaymentMethodDTO {
	return &dto.PaymentMethodDTO{UserID: userID, Name: name}
}

func TestPaymentMethodCreateGetRoundtrip(t *testing.T) {
	repo := NewPaymentMethodRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"

	created, err := repo.Create(ctx, dtoPaymentMethod(userID, "cash"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, userID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "cash" {
		t.Errorf("got %+v", got)
	}
}

func TestPaymentMethodEnsureByNameIsIdempotent(t *testing.T) {
	repo := NewPaymentMethodRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"

	first, err := repo.EnsureByName(ctx, userID, "pix")
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.EnsureByName(ctx, userID, "Pix") // case-insensitive match
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
		t.Errorf("want exactly one payment method after two EnsureByName calls for the same name, got %d", len(all))
	}
}

// TestPaymentMethodCreateRejectsDuplicateName guards the unique
// (user_id, lower(name)) index the migration adds — the application
// layer's list-then-compare check is the common-path guard, but the DB
// constraint is the actual backstop against a race creating two rows
// for the same name.
func TestPaymentMethodCreateRejectsDuplicateName(t *testing.T) {
	repo := NewPaymentMethodRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"

	if _, err := repo.Create(ctx, dtoPaymentMethod(userID, "cash")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, dtoPaymentMethod(userID, "Cash")); err == nil {
		t.Error("want an error inserting a case-variant duplicate name, got nil")
	}
}

func TestPaymentMethodListByUserScopesToOwner(t *testing.T) {
	repo := NewPaymentMethodRepository(openTestDB(t))
	ctx := context.Background()

	if _, err := repo.Create(ctx, dtoPaymentMethod("u1", "cash")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, dtoPaymentMethod("u2", "debit_card")); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListByUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "cash" {
		t.Errorf("want exactly [cash] for u1, got %+v", got)
	}
}

func TestPaymentMethodUpdateRenames(t *testing.T) {
	repo := NewPaymentMethodRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"

	created, err := repo.Create(ctx, dtoPaymentMethod(userID, "cash"))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Update(ctx, userID, created.ID, "physical cash"); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, userID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "physical cash" {
		t.Errorf("got %+v", got)
	}
}

func TestPaymentMethodUpdateNotFound(t *testing.T) {
	repo := NewPaymentMethodRepository(openTestDB(t))
	ctx := context.Background()

	err := repo.Update(ctx, "u1", "missing-id", "whatever")
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestPaymentMethodDeleteRemovesRow(t *testing.T) {
	repo := NewPaymentMethodRepository(openTestDB(t))
	ctx := context.Background()
	userID := "u1"

	created, err := repo.Create(ctx, dtoPaymentMethod(userID, "cash"))
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

func TestPaymentMethodDeleteNotFound(t *testing.T) {
	repo := NewPaymentMethodRepository(openTestDB(t))
	ctx := context.Background()

	err := repo.Delete(ctx, "u1", "missing-id")
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}
