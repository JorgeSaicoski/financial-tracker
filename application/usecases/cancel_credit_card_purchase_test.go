package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

// seedPurchase creates a purchase whose first installment is already
// synced to ledger-service and the rest are still pending (the usual
// shape: only due installments sync).
func seedPurchase(t *testing.T, movements *fakeMovementRepo, purchases *fakePurchaseRepo) *dto.CreditCardPurchaseDTO {
	t.Helper()
	uc := NewCreateCreditCardPurchase(purchases)
	purchase, installments, err := uc.Execute(context.Background(), CreateCreditCardPurchaseInput{
		UserID: "u1", TotalAmount: -900, Currency: "usd", Installments: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := movements.MarkSynced(context.Background(), installments[0].ID, "ledger-1", installments[0].Timestamp); err != nil {
		t.Fatal(err)
	}
	return purchase
}

func TestCancelPurchaseReversesSyncedAndVoidsRest(t *testing.T) {
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)
	trigger := &fakeSyncTrigger{}
	purchase := seedPurchase(t, movements, purchases)

	result, err := NewCancelCreditCardPurchase(purchases, movements, trigger).Execute(context.Background(), purchase.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Reversals) != 1 {
		t.Fatalf("reversals = %d, want 1 (only the synced installment)", len(result.Reversals))
	}
	if result.Reversals[0].Amount != 300 {
		t.Errorf("reversal amount = %d, want 300", result.Reversals[0].Amount)
	}
	if result.Reversals[0].CreditCardPurchaseID != nil {
		t.Error("a reversal must not join the purchase's installment set")
	}
	if len(result.Voided) != 2 {
		t.Fatalf("voided = %d, want 2 (the unsynced installments)", len(result.Voided))
	}
	if result.Purchase.Status != string(entities.CreditCardPurchaseStatusCancelled) {
		t.Errorf("purchase status = %q, want cancelled", result.Purchase.Status)
	}
	if trigger.calls != 1 {
		t.Errorf("sync trigger calls = %d, want 1", trigger.calls)
	}

	stored, _ := purchases.GetByID(context.Background(), purchase.ID)
	if stored.Status != string(entities.CreditCardPurchaseStatusCancelled) {
		t.Error("purchase not persisted as cancelled")
	}
}

func TestCancelPurchaseSkipsAlreadyCancelledInstallments(t *testing.T) {
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)
	trigger := &fakeSyncTrigger{}
	purchase := seedPurchase(t, movements, purchases)

	// Cancel the synced installment individually first.
	installments, _ := movements.ListByCreditCardPurchase(context.Background(), purchase.ID)
	var syncedID string
	for _, m := range installments {
		if m.ToEntity().IsSynced() {
			syncedID = m.ID
		}
	}
	if _, err := NewCancelMovement(movements, trigger).Execute(context.Background(), syncedID); err != nil {
		t.Fatal(err)
	}

	result, err := NewCancelCreditCardPurchase(purchases, movements, trigger).Execute(context.Background(), purchase.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Reversals) != 0 {
		t.Errorf("already-reversed installment reversed again: %d reversals", len(result.Reversals))
	}
	if len(result.Voided) != 2 {
		t.Errorf("voided = %d, want 2", len(result.Voided))
	}
}

func TestCancelPurchaseRejectsRepeatAndMissing(t *testing.T) {
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)
	trigger := &fakeSyncTrigger{}
	purchase := seedPurchase(t, movements, purchases)
	uc := NewCancelCreditCardPurchase(purchases, movements, trigger)

	if _, err := uc.Execute(context.Background(), purchase.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Execute(context.Background(), purchase.ID); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("second cancel: want ErrConflict, got %v", err)
	}
	if _, err := uc.Execute(context.Background(), "nope"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("missing purchase: want ErrNotFound, got %v", err)
	}
}
