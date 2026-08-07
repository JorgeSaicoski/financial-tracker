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
uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), newFakeCategoryRepo(), newFakeUserSettingsRepo())

	cases := []struct {
		name  string
		input CreateMovementInput
	}{
		{"missing user", CreateMovementInput{Amount: 100, Currency: "usd"}},
		{"missing currency", CreateMovementInput{UserID: "u1", Amount: 100}},
		{"zero amount", CreateMovementInput{UserID: "u1", Currency: "usd"}},
		// "unknown payment method" is intentionally absent (BACK-17): an
		// unrecognized payment method is no longer rejected, it's
		// implicitly registered — see
		// TestCreateMovementImplicitlyRegistersPaymentMethod below.
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

// TestCreateMovementImplicitlyRegistersPaymentMethod covers BACK-17's
// replacement for the old fixed-enum rejection: a payment method the
// caller has never used before ("iou") is no longer invalid — it's
// implicitly registered, same idempotent-Add shape as a brand-new
// category or currency.
func TestCreateMovementImplicitlyRegistersPaymentMethod(t *testing.T) {
	methods := newFakePaymentMethodRepo()
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), methods, newFakeCategoryRepo(), newFakeUserSettingsRepo())

	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: 100, Currency: "usd", PaymentMethod: "iou",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.PaymentMethod != "iou" {
		t.Errorf("payment method = %q, want iou", m.PaymentMethod)
	}
	rows, err := methods.ListByUser(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Name != "iou" {
		t.Errorf("want exactly one implicitly-registered payment method %q, got %+v", "iou", rows)
	}
}

// TestCreateMovementNormalizesPaymentMethodCasing guards against exactly
// the drift the review flagged: resolvePaymentMethod used to echo back
// the caller's raw-cased input after EnsureByName instead of the
// registry's canonical stored name. Left unfixed, a case-variant of a
// reserved system name ("Credit_Card") would get stored verbatim on the
// movement, breaking exact-string comparisons against
// entities.PaymentMethodCreditCard elsewhere.
func TestCreateMovementNormalizesPaymentMethodCasing(t *testing.T) {
	methods := newFakePaymentMethodRepo()
	if _, err := methods.EnsureByName(context.Background(), "u1", "credit_card"); err != nil {
		t.Fatal(err)
	}
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), methods, newFakeCategoryRepo(), newFakeUserSettingsRepo())

	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: 100, Currency: "usd", PaymentMethod: "Credit_Card",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.PaymentMethod != string(entities.PaymentMethodCreditCard) {
		t.Errorf("payment method = %q, want canonical %q", m.PaymentMethod, entities.PaymentMethodCreditCard)
	}
}

func TestCreateMovementDefaultsAndState(t *testing.T) {
	repo := newFakeMovementRepo()
uc := NewCreateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), newFakeCategoryRepo(), newFakeUserSettingsRepo())

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
	if m.PaymentMethod != "other" {
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
uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), newFakeCategoryRepo(), newFakeUserSettingsRepo())

	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd",
		Description:   "groceries",
		PaymentMethod: "pix",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Description != "groceries" || m.PaymentMethod != "pix" {
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
	food, err := categories.Create(context.Background(), &dto.CategoryDTO{Name: "food", AvoidabilityPercent: &avoidability, ContributorIDs: []string{"u1"}})
	if err != nil {
		t.Fatal(err)
	}
uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), categories, newFakeUserSettingsRepo())

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

// TestCreateMovementRejectsCategoryUserDoesNotHave guards the actual gap
// the PR #39 review found: resolveCategoryID must check the caller
// currently has category_id (CategoryRepository.HasForUser), not just
// that it exists somewhere — otherwise a category removed from a user's
// own list could still be selected on a brand-new movement, exactly
// backwards from "he can't choose more times restaurant" (Jorge,
// 2026-07-28).
func TestCreateMovementRejectsCategoryUserDoesNotHave(t *testing.T) {
	categories := newFakeCategoryRepo()
	// "food" belongs to u2, not u1.
	food, err := categories.Create(context.Background(), &dto.CategoryDTO{Name: "food", ContributorIDs: []string{"u2"}})
	if err != nil {
		t.Fatal(err)
	}
uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), categories, newFakeUserSettingsRepo())

	_, err = uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd", CategoryID: &food.ID,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput selecting a category u1 doesn't have, got %v", err)
	}
}

// TestCreateMovementRejectsCategoryRemovedFromCallersList is the exact
// scenario from the review: create with "restaurant", remove it from the
// list, then try to use it again on a new movement.
func TestCreateMovementRejectsCategoryRemovedFromCallersList(t *testing.T) {
	categories := newFakeCategoryRepo()
	restaurant, err := categories.Create(context.Background(), &dto.CategoryDTO{Name: "restaurant", ContributorIDs: []string{"u1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := categories.Remove(context.Background(), "u1", restaurant.ID); err != nil {
		t.Fatal(err)
	}
uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), categories, newFakeUserSettingsRepo())

	_, err = uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd", CategoryID: &restaurant.ID,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput re-selecting a removed category, got %v", err)
	}
}

// TestCreateMovementAllowsSystemCategoryRegardlessOfHasList guards the
// bypass resolveCategoryID must apply for transfer/income/other — no
// user_categories row is ever seeded for them, so without the bypass
// nobody could ever select them explicitly.
func TestCreateMovementAllowsSystemCategoryRegardlessOfHasList(t *testing.T) {
	categories := newFakeCategoryRepo()
	if _, err := categories.Create(context.Background(), &dto.CategoryDTO{ID: entities.CategoryOtherID, Name: entities.CategoryOther}); err != nil {
		t.Fatal(err)
	}
uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakePlanRepo(), categories, newFakeUserSettingsRepo())

	otherID := entities.CategoryOtherID
	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd", CategoryID: &otherID,
	})
	if err != nil {
		t.Fatalf("unexpected error selecting the system \"other\" category: %v", err)
	}
	if m.CategoryID == nil || *m.CategoryID != entities.CategoryOtherID {
		t.Errorf("category_id = %v, want %q", m.CategoryID, entities.CategoryOtherID)
	}
}
