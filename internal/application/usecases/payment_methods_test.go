package usecases

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestCreatePaymentMethodRejectsSystemNames(t *testing.T) {
	uc := NewCreatePaymentMethod(newFakePaymentMethodRepo())

	for _, name := range []string{"credit_card", "bank_transfer", "Credit_Card"} {
		if _, err := uc.Execute(context.Background(), CreatePaymentMethodInput{UserID: "u1", Name: name}); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Errorf("name %q: want ErrInvalidInput, got %v", name, err)
		}
	}
}

func TestCreatePaymentMethodRejectsDuplicateName(t *testing.T) {
	methods := newFakePaymentMethodRepo()
	uc := NewCreatePaymentMethod(methods)

	if _, err := uc.Execute(context.Background(), CreatePaymentMethodInput{UserID: "u1", Name: "voucher"}); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Execute(context.Background(), CreatePaymentMethodInput{UserID: "u1", Name: "Voucher"}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("want ErrConflict for a case-variant duplicate, got %v", err)
	}
}

func TestCreatePaymentMethodRejectsEmptyName(t *testing.T) {
	uc := NewCreatePaymentMethod(newFakePaymentMethodRepo())
	if _, err := uc.Execute(context.Background(), CreatePaymentMethodInput{UserID: "u1", Name: "  "}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

// TestListPaymentMethodsSeedsSystemAndDefaultEntries covers the ticket's
// own acceptance criterion: a fresh user's registry contains cash,
// debit_card, credit_card, bank_transfer, other — and explicitly not pix.
func TestListPaymentMethodsSeedsSystemAndDefaultEntries(t *testing.T) {
	methods := newFakePaymentMethodRepo()
	uc := NewListPaymentMethods(methods)

	rows, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}

	names := make(map[string]bool, len(rows))
	for _, r := range rows {
		names[r.Name] = true
	}
	for _, want := range []string{"cash", "debit_card", "credit_card", "bank_transfer", "other"} {
		if !names[want] {
			t.Errorf("want %q in a fresh user's payment methods, got %+v", want, names)
		}
	}
	if names["pix"] {
		t.Error("pix must not be seeded by default — it's Brazil-specific, users add it themselves")
	}

	// Idempotent: a second call doesn't duplicate the system/default rows.
	rows2, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows2) != len(rows) {
		t.Errorf("second ListPaymentMethods call changed the row count: %d vs %d", len(rows2), len(rows))
	}
}

func TestUpdatePaymentMethodRejectsSystemEntries(t *testing.T) {
	methods := newFakePaymentMethodRepo()
	uc := NewUpdatePaymentMethod(methods)

	creditCard, err := methods.EnsureByName(context.Background(), "u1", "credit_card")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Execute(context.Background(), "u1", creditCard.ID, UpdatePaymentMethodInput{Name: "cc"}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput editing a system entry, got %v", err)
	}
}

func TestUpdatePaymentMethodRenamesOrdinaryEntry(t *testing.T) {
	methods := newFakePaymentMethodRepo()
	createUC := NewCreatePaymentMethod(methods)
	updateUC := NewUpdatePaymentMethod(methods)

	created, err := createUC.Execute(context.Background(), CreatePaymentMethodInput{UserID: "u1", Name: "voucher"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := updateUC.Execute(context.Background(), "u1", created.ID, UpdatePaymentMethodInput{Name: "meal voucher"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "meal voucher" {
		t.Errorf("name = %q, want %q", updated.Name, "meal voucher")
	}
}

func TestUpdatePaymentMethodRejectsRenameOntoExistingName(t *testing.T) {
	methods := newFakePaymentMethodRepo()
	createUC := NewCreatePaymentMethod(methods)
	updateUC := NewUpdatePaymentMethod(methods)

	if _, err := createUC.Execute(context.Background(), CreatePaymentMethodInput{UserID: "u1", Name: "voucher"}); err != nil {
		t.Fatal(err)
	}
	other, err := createUC.Execute(context.Background(), CreatePaymentMethodInput{UserID: "u1", Name: "gift card"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := updateUC.Execute(context.Background(), "u1", other.ID, UpdatePaymentMethodInput{Name: "Voucher"}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("want ErrConflict renaming onto an existing name, got %v", err)
	}
}

func TestDeletePaymentMethodRejectsSystemEntries(t *testing.T) {
	methods := newFakePaymentMethodRepo()
	uc := NewDeletePaymentMethod(methods)

	bankTransfer, err := methods.EnsureByName(context.Background(), "u1", "bank_transfer")
	if err != nil {
		t.Fatal(err)
	}
	if err := uc.Execute(context.Background(), "u1", bankTransfer.ID); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput deleting a system entry, got %v", err)
	}
}

func TestDeletePaymentMethodRemovesOrdinaryEntry(t *testing.T) {
	methods := newFakePaymentMethodRepo()
	createUC := NewCreatePaymentMethod(methods)
	deleteUC := NewDeletePaymentMethod(methods)

	created, err := createUC.Execute(context.Background(), CreatePaymentMethodInput{UserID: "u1", Name: "voucher"})
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteUC.Execute(context.Background(), "u1", created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := methods.GetByID(context.Background(), "u1", created.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}
