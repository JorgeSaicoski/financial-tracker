package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

func newPurchaseUseCase() (CreateCreditCardPurchaseUseCase, *fakeMovementRepo) {
	movements := newFakeMovementRepo()
	return NewCreateCreditCardPurchase(newFakePurchaseRepo(movements)), movements
}

func TestCreatePurchaseValidation(t *testing.T) {
	uc, _ := newPurchaseUseCase()

	cases := []struct {
		name  string
		input CreateCreditCardPurchaseInput
	}{
		{"one installment", CreateCreditCardPurchaseInput{UserID: "u1", TotalAmount: -1000, Currency: "usd", Installments: 1}},
		{"zero amount", CreateCreditCardPurchaseInput{UserID: "u1", Currency: "usd", Installments: 3}},
		{"too small to split", CreateCreditCardPurchaseInput{UserID: "u1", TotalAmount: -5, Currency: "usd", Installments: 12}},
		{"unknown category", CreateCreditCardPurchaseInput{UserID: "u1", TotalAmount: -1000, Currency: "usd", Installments: 3, Category: "yacht"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := uc.Execute(context.Background(), tc.input); !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Fatalf("want ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestCreatePurchaseSplitsAmountExactly(t *testing.T) {
	cases := []struct {
		name  string
		total int64
		n     int
		want  []int64
	}{
		{"expense with remainder", -1000, 3, []int64{-333, -333, -334}},
		{"income with remainder", 1000, 3, []int64{333, 333, 334}},
		{"even split", -1200, 4, []int64{-300, -300, -300, -300}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc, _ := newPurchaseUseCase()
			purchase, installments, err := uc.Execute(context.Background(), CreateCreditCardPurchaseInput{
				UserID: "u1", TotalAmount: tc.total, Currency: "usd", Installments: tc.n,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var sum int64
			for i, m := range installments {
				if m.Amount != tc.want[i] {
					t.Errorf("installment %d amount = %d, want %d", i+1, m.Amount, tc.want[i])
				}
				sum += m.Amount
			}
			if sum != tc.total {
				t.Errorf("installments sum to %d, want %d", sum, tc.total)
			}
			if purchase.TotalAmount != tc.total || purchase.InstallmentCount != tc.n {
				t.Errorf("purchase record mismatch: %+v", purchase)
			}
		})
	}
}

func TestCreatePurchaseInstallmentShape(t *testing.T) {
	uc, movements := newPurchaseUseCase()
	purchase, installments, err := uc.Execute(context.Background(), CreateCreditCardPurchaseInput{
		UserID: "u1", TotalAmount: -900, Currency: "usd", Installments: 3, Description: "tv",
		Category: entities.CategoryShopping,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, m := range installments {
		if m.PaymentMethod != entities.PaymentMethodCreditCard {
			t.Errorf("installment %d payment method = %q", i+1, m.PaymentMethod)
		}
		if m.InstallmentNumber == nil || *m.InstallmentNumber != i+1 {
			t.Errorf("installment %d has number %v", i+1, m.InstallmentNumber)
		}
		if m.CreditCardPurchaseID == nil || *m.CreditCardPurchaseID != purchase.ID {
			t.Errorf("installment %d not linked to purchase", i+1)
		}
		if m.SyncStatus != entities.SyncStatusPending || m.Status != entities.MovementStatusActive {
			t.Errorf("installment %d state = %s/%s", i+1, m.Status, m.SyncStatus)
		}
		// One per month starting the purchase month.
		want := purchase.PurchaseDate.AddDate(0, i, 0)
		if !m.Timestamp.Equal(want) {
			t.Errorf("installment %d timestamp = %s, want %s", i+1, m.Timestamp, want)
		}
	}

	stored, err := movements.ListByCreditCardPurchase(context.Background(), purchase.ID)
	if err != nil || len(stored) != 3 {
		t.Fatalf("stored %d installments (err %v), want 3", len(stored), err)
	}
}
