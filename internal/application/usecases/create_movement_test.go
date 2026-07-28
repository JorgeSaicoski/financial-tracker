package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestCreateMovementValidation(t *testing.T) {
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), newFakeCardRepo(), newFakePaymentMethodRepo(), newFakeUserSettingsRepo())

	cases := []struct {
		name  string
		input CreateMovementInput
	}{
		{"missing user", CreateMovementInput{Amount: 100, Currency: "usd"}},
		{"missing currency", CreateMovementInput{UserID: "u1", Amount: 100}},
		{"zero amount", CreateMovementInput{UserID: "u1", Currency: "usd"}},
		{"unknown category", CreateMovementInput{UserID: "u1", Amount: 100, Currency: "usd", Category: "yacht"}},
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
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), newFakeCardRepo(), methods, newFakeUserSettingsRepo())

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

func TestCreateMovementDefaultsAndState(t *testing.T) {
	repo := newFakeMovementRepo()
	uc := NewCreateMovement(repo, newFakeAccountRepo(), newFakeCardRepo(), newFakePaymentMethodRepo(), newFakeUserSettingsRepo())

	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.Category != string(entities.CategoryOther) {
		t.Errorf("category = %q, want other", m.Category)
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
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), newFakeCardRepo(), newFakePaymentMethodRepo(), newFakeUserSettingsRepo())

	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd",
		Description:   "groceries",
		Category:      string(entities.CategoryFood),
		PaymentMethod: "pix",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Description != "groceries" || m.Category != string(entities.CategoryFood) || m.PaymentMethod != "pix" {
		t.Errorf("fields not preserved: %+v", m)
	}
}

func TestCreateMovementWithCardIDDatesOnDueDay(t *testing.T) {
	cards := newFakeCardRepo()
	card, err := cards.Create(context.Background(), dtoCard("u1", "5", "15"))
	if err != nil {
		t.Fatal(err)
	}
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), cards, newFakePaymentMethodRepo(), newFakeUserSettingsRepo())

	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd",
		PaymentMethod: string(entities.PaymentMethodCreditCard), CardID: &card.ID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.CardID == nil || *m.CardID != card.ID {
		t.Errorf("card_id not set: %+v", m)
	}
	wantDue := entities.NextCardDueDate(card.ClosingDay, card.DueDay, m.CreatedAt)
	if !m.Timestamp.Equal(wantDue) {
		t.Errorf("timestamp = %v, want the card's due date %v (not \"now\")", m.Timestamp, wantDue)
	}
}

func TestCreateMovementCardIDRequiresCreditCardPaymentMethod(t *testing.T) {
	cards := newFakeCardRepo()
	card, err := cards.Create(context.Background(), dtoCard("u1", "5", "15"))
	if err != nil {
		t.Fatal(err)
	}
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), cards, newFakePaymentMethodRepo(), newFakeUserSettingsRepo())

	_, err = uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "usd", CardID: &card.ID,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput when card_id is set without payment_method=credit_card, got %v", err)
	}
}

func TestCreateMovementCardIDRejectsCurrencyMismatch(t *testing.T) {
	cards := newFakeCardRepo()
	card, err := cards.Create(context.Background(), dtoCard("u1", "5", "15"))
	if err != nil {
		t.Fatal(err)
	}
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), cards, newFakePaymentMethodRepo(), newFakeUserSettingsRepo())

	_, err = uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -500, Currency: "brl",
		PaymentMethod: string(entities.PaymentMethodCreditCard), CardID: &card.ID,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for currency mismatch, got %v", err)
	}
}

func TestCreateMovementCardPaymentKeepsNowAsTimestamp(t *testing.T) {
	cards := newFakeCardRepo()
	card, err := cards.Create(context.Background(), dtoCard("u1", "5", "15"))
	if err != nil {
		t.Fatal(err)
	}
	uc := NewCreateMovement(newFakeMovementRepo(), newFakeAccountRepo(), cards, newFakePaymentMethodRepo(), newFakeUserSettingsRepo())

	before := time.Now().UTC()
	m, err := uc.Execute(context.Background(), CreateMovementInput{
		UserID: "u1", Amount: -400, Currency: "usd",
		Category: string(entities.CategoryTransfer), CardPaymentForCardID: &card.ID,
	})
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.CardPaymentForCardID == nil || *m.CardPaymentForCardID != card.ID {
		t.Errorf("card_payment_for_card_id not set: %+v", m)
	}
	if m.Timestamp.Before(before) || m.Timestamp.After(after) {
		t.Errorf("a card payment's timestamp should be \"now\", got %v (window %v..%v)", m.Timestamp, before, after)
	}
}

func dtoCard(userID, closingDay, dueDay string) *dto.CardDTO {
	return &dto.CardDTO{UserID: userID, Name: "Visa", ClosingDay: closingDay, DueDay: dueDay, Currency: "usd"}
}
