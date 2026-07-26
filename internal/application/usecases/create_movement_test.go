package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestCreateMovementValidation(t *testing.T) {
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), newFakeCategoryRepo(), newFakeUserSettingsRepo())

	cases := []struct {
		name  string
		input CreateMovementInput
	}{
		{"missing user", CreateMovementInput{Amount: 100, Currency: "usd"}},
		{"missing currency", CreateMovementInput{UserID: "u1", Amount: 100}},
		{"zero amount", CreateMovementInput{UserID: "u1", Currency: "usd"}},
		{"unknown payment method", CreateMovementInput{UserID: "u1", Amount: 100, Currency: "usd", PaymentMethod: "iou"}},
		{"avoidability_percent out of range", CreateMovementInput{UserID: "u1", Amount: 100, Currency: "usd", AvoidabilityOverridePercent: intPtrAv(101)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := uc.Execute(context.Background(), tc.input); !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Fatalf("want ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestCreateMovementDefaultsAndState(t *testing.T) {
	repo := newFakeMovementRepo()
	uc := NewCreateMovement(repo, newFakeAccountRepo(), newFakeCategoryRepo(), newFakeUserSettingsRepo())

	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// BACK-14: category no longer defaults to "other" — empty stays
	// genuinely uncategorized, pairing with the ad-hoc avoidability
	// override for one-off spends that don't deserve a category.
	if m.Category != "" {
		t.Errorf("category = %q, want empty (uncategorized)", m.Category)
	}
	if m.PaymentMethod != string(entities.PaymentMethodOther) {
		t.Errorf("payment method = %q, want other", m.PaymentMethod)
	}
	if m.Status != string(entities.MovementStatusActive) {
		t.Errorf("status = %q, want active", m.Status)
	}
	if m.SyncStatus != string(entities.SyncStatusPending) {
		t.Errorf("sync status = %q, want pending", m.SyncStatus)
	}
	if m.Timestamp.IsZero() || m.CreatedAt.IsZero() {
		t.Error("timestamps should be set")
	}
}

func TestCreateMovementKeepsExplicitFields(t *testing.T) {
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), newFakeCategoryRepo(), newFakeUserSettingsRepo())

	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd",
		Description:   "groceries",
		Category:      "food",
		PaymentMethod: string(entities.PaymentMethodPix),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Description != "groceries" || m.Category != "food" || m.PaymentMethod != string(entities.PaymentMethodPix) {
		t.Errorf("fields not preserved: %+v", m)
	}
}

// TestCreateMovementImplicitlyRegistersNewCategory guards BACK-14's
// registry behavior: a brand-new category name auto-registers at a
// neutral 50% default on first use, no separate create call required —
// same idempotent-Add shape as AddCurrencyUseCase.
func TestCreateMovementImplicitlyRegistersNewCategory(t *testing.T) {
	categories := newFakeCategoryRepo()
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), categories, newFakeUserSettingsRepo())

	if _, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd", Category: "kayaking",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rows, err := categories.ListByUser(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "kayaking" {
		t.Fatalf("categories = %+v, want one row named kayaking", rows)
	}
	if rows[0].AvoidabilityPercent == nil || *rows[0].AvoidabilityPercent != defaultCategoryAvoidability {
		t.Errorf("avoidability_percent = %v, want %d", rows[0].AvoidabilityPercent, defaultCategoryAvoidability)
	}

	// A second movement with the same category name doesn't create a
	// duplicate row.
	if _, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -100, Currency: "usd", Category: "Kayaking", // case-insensitive match
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rows, _ = categories.ListByUser(context.Background(), "u1")
	if len(rows) != 1 {
		t.Errorf("got %d category rows, want still 1 (no duplicate)", len(rows))
	}
}
