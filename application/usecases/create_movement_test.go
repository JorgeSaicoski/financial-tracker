package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

func TestCreateMovementValidation(t *testing.T) {
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo())

	cases := []struct {
		name  string
		input CreateMovementInput
	}{
		{"missing user", CreateMovementInput{Amount: 100, Currency: "usd"}},
		{"missing currency", CreateMovementInput{UserID: "u1", Amount: 100}},
		{"zero amount", CreateMovementInput{UserID: "u1", Currency: "usd"}},
		{"unknown category", CreateMovementInput{UserID: "u1", Amount: 100, Currency: "usd", Category: "yacht"}},
		{"unknown payment method", CreateMovementInput{UserID: "u1", Amount: 100, Currency: "usd", PaymentMethod: "iou"}},
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
	uc := NewCreateMovement(repo, newFakeAccountRepo())

	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.Category != entities.CategoryOther {
		t.Errorf("category = %q, want other", m.Category)
	}
	if m.PaymentMethod != entities.PaymentMethodOther {
		t.Errorf("payment method = %q, want other", m.PaymentMethod)
	}
	if m.Status != entities.MovementStatusActive {
		t.Errorf("status = %q, want active", m.Status)
	}
	if m.SyncStatus != entities.SyncStatusPending {
		t.Errorf("sync status = %q, want pending", m.SyncStatus)
	}
	if m.Timestamp.IsZero() || m.CreatedAt.IsZero() {
		t.Error("timestamps should be set")
	}
}

func TestCreateMovementKeepsExplicitFields(t *testing.T) {
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo())

	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd",
		Description:   "groceries",
		Category:      entities.CategoryFood,
		PaymentMethod: entities.PaymentMethodPix,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Description != "groceries" || m.Category != entities.CategoryFood || m.PaymentMethod != entities.PaymentMethodPix {
		t.Errorf("fields not preserved: %+v", m)
	}
}
