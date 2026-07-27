package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
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
		{"unknown category_id", CreateMovementInput{UserID: "u1", Amount: 100, Currency: "usd", CategoryID: strPtrAv("nope")}},
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

	// BACK-14 follow-up: category_id no longer defaults to anything —
	// nil stays genuinely uncategorized, pairing with the ad-hoc
	// avoidability override for one-off spends that don't deserve a
	// category.
	if m.CategoryID != nil {
		t.Errorf("category_id = %v, want nil (uncategorized)", m.CategoryID)
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
		PaymentMethod: string(entities.PaymentMethodPix),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Description != "groceries" || m.PaymentMethod != string(entities.PaymentMethodPix) {
		t.Errorf("fields not preserved: %+v", m)
	}
}

// TestCreateMovementRequiresExistingCategoryID guards BACK-14 follow-up's
// contract: category_id must reference an existing (globally visible)
// category — there's no more implicit name-based registration, since
// names aren't unique anymore.
func TestCreateMovementRequiresExistingCategoryID(t *testing.T) {
	categories := newFakeCategoryRepo()
	avoidability := 50
	food, err := categories.Create(context.Background(), &dto.CategoryDTO{Name: "food", AvoidabilityPercent: &avoidability})
	if err != nil {
		t.Fatal(err)
	}
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), categories, newFakeUserSettingsRepo())

	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd", CategoryID: &food.ID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.CategoryID == nil || *m.CategoryID != food.ID {
		t.Errorf("category_id = %v, want %q", m.CategoryID, food.ID)
	}
}
