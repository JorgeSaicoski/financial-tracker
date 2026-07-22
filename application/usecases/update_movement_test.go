package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

func strPtr(s string) *string { return &s }
func int64Ptr(n int64) *int64 { return &n }

func TestUpdateMovementMetadataOnSyncedMovementEditsInPlace(t *testing.T) {
	repo := newFakeMovementRepo()
	trigger := &fakeSyncTrigger{}
	repo.add(activeMovement("m1", -500, entities.SyncStatusSynced))

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), trigger)
	newDescription := "corrected description"
	newCategory := entities.CategoryTransport
	result, err := uc.Execute(context.Background(), "m1", UpdateMovementInput{
		Description: &newDescription,
		Category:    &newCategory,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reversal != nil || result.Replacement != nil {
		t.Errorf("metadata-only edit must not reverse/recreate: %+v", result)
	}
	if result.Movement.Description != newDescription || result.Movement.Category != newCategory {
		t.Errorf("metadata not applied: %+v", result.Movement)
	}
	if result.Movement.Amount != -500 {
		t.Errorf("amount changed unexpectedly: %d", result.Movement.Amount)
	}
	if trigger.calls != 0 {
		t.Error("metadata-only edit must not trigger a sync")
	}

	stored, _ := repo.GetByID(context.Background(), "m1")
	if stored.Description != newDescription || stored.Category != newCategory {
		t.Errorf("metadata not persisted: %+v", stored)
	}
}

func TestUpdateMovementAmountPreSyncEditsInPlace(t *testing.T) {
	repo := newFakeMovementRepo()
	trigger := &fakeSyncTrigger{}
	repo.add(activeMovement("m1", -500, entities.SyncStatusPending))

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), trigger)
	result, err := uc.Execute(context.Background(), "m1", UpdateMovementInput{
		Amount: int64Ptr(-750),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reversal != nil || result.Replacement != nil {
		t.Errorf("pre-sync edit must not reverse/recreate: %+v", result)
	}
	if result.Movement.Amount != -750 {
		t.Errorf("amount = %d, want -750", result.Movement.Amount)
	}
	if trigger.calls != 0 {
		t.Error("pre-sync in-place edit must not trigger a sync")
	}

	stored, _ := repo.GetByID(context.Background(), "m1")
	if stored.Amount != -750 {
		t.Errorf("amount not persisted: %+v", stored)
	}
}

func TestUpdateMovementPreSyncRollsBackFinancialUpdateWhenMetadataUpdateFails(t *testing.T) {
	repo := newFakeMovementRepo()
	trigger := &fakeSyncTrigger{}
	repo.add(activeMovement("m1", -500, entities.SyncStatusPending))
	repo.updateMetadataErr = errors.New("metadata failed")

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), trigger)
	newDescription := "corrected"
	_, err := uc.Execute(context.Background(), "m1", UpdateMovementInput{
		Amount:      int64Ptr(-750),
		Description: &newDescription,
	})
	if !errors.Is(err, repo.updateMetadataErr) {
		t.Fatalf("want metadata error, got %v", err)
	}
	if trigger.calls != 0 {
		t.Error("failed pre-sync edit must not trigger a sync")
	}

	stored, _ := repo.GetByID(context.Background(), "m1")
	if stored.Amount != -500 {
		t.Errorf("amount changed despite rollback: %+v", stored)
	}
	if stored.Description != "" {
		t.Errorf("metadata changed unexpectedly: %+v", stored)
	}
}

func TestUpdateMovementAmountPostSyncReversesAndRecreates(t *testing.T) {
	repo := newFakeMovementRepo()
	trigger := &fakeSyncTrigger{}
	repo.add(activeMovement("kept", -100, entities.SyncStatusSynced))
	repo.add(activeMovement("m1", -500, entities.SyncStatusSynced))

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), trigger)
	result, err := uc.Execute(context.Background(), "m1", UpdateMovementInput{
		Amount: int64Ptr(-750),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reversal == nil || result.Replacement == nil {
		t.Fatalf("post-sync edit must reverse + recreate: %+v", result)
	}
	if result.Reversal.Amount != 500 {
		t.Errorf("reversal amount = %d, want 500", result.Reversal.Amount)
	}
	if result.Reversal.CancelsMovementID == nil || *result.Reversal.CancelsMovementID != "m1" {
		t.Error("reversal not linked to original")
	}
	if result.Replacement.Amount != -750 {
		t.Errorf("replacement amount = %d, want -750", result.Replacement.Amount)
	}
	if result.Replacement.SyncStatus != entities.SyncStatusPending {
		t.Errorf("replacement sync status = %s, want pending", result.Replacement.SyncStatus)
	}
	if trigger.calls != 1 {
		t.Errorf("sync trigger calls = %d, want 1", trigger.calls)
	}

	original, _ := repo.GetByID(context.Background(), "m1")
	if original.Amount != -500 {
		t.Errorf("original amount must stay untouched, got %d", original.Amount)
	}
	if original.ReversedByMovementID == nil || *original.ReversedByMovementID != result.Reversal.ID {
		t.Error("original not linked to its reversal")
	}

	// Balance ends correct: kept (-100) + original (-500) + reversal
	// (+500) + replacement (-750) = -850.
	list := NewListMovements(repo)
	balance, err := list.Execute(context.Background(), "u1", nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Balance != -850 {
		t.Errorf("balance = %d, want -850", balance.Balance)
	}
}

func TestUpdateMovementRejectsVoidedMovement(t *testing.T) {
	repo := newFakeMovementRepo()
	voided := activeMovement("m1", -500, entities.SyncStatusPending)
	voided.Status = entities.MovementStatusVoided
	repo.add(voided)

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), "m1", UpdateMovementInput{Amount: int64Ptr(-1)}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestUpdateMovementRejectsReversedMovement(t *testing.T) {
	repo := newFakeMovementRepo()
	reversedID := "rev"
	reversed := activeMovement("m1", -500, entities.SyncStatusSynced)
	reversed.ReversedByMovementID = &reversedID
	repo.add(reversed)

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), "m1", UpdateMovementInput{Amount: int64Ptr(-1)}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestUpdateMovementRejectsReversalItself(t *testing.T) {
	repo := newFakeMovementRepo()
	originalID := "original"
	reversal := activeMovement("rev", 500, entities.SyncStatusPending)
	reversal.CancelsMovementID = &originalID
	repo.add(reversal)

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), &fakeSyncTrigger{})
	newDescription := "nope"
	if _, err := uc.Execute(context.Background(), "rev", UpdateMovementInput{Description: &newDescription}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestUpdateMovementRejectsInstallmentFinancialEdit(t *testing.T) {
	repo := newFakeMovementRepo()
	purchaseID := "p1"
	installment := activeMovement("m1", -300, entities.SyncStatusPending)
	installment.CreditCardPurchaseID = &purchaseID
	repo.add(installment)

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), "m1", UpdateMovementInput{Amount: int64Ptr(-400)}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}

	// Metadata edits on installments are still fine.
	newDescription := "renamed"
	result, err := uc.Execute(context.Background(), "m1", UpdateMovementInput{Description: &newDescription})
	if err != nil {
		t.Fatalf("metadata edit on installment should succeed: %v", err)
	}
	if result.Movement.Description != newDescription {
		t.Errorf("description not applied: %+v", result.Movement)
	}
}

func TestUpdateMovementValidatesLikeCreate(t *testing.T) {
	repo := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	repo.add(activeMovement("m1", -500, entities.SyncStatusPending))
	account, _ := accounts.Create(context.Background(), &entities.Account{UserID: "someone-else", Currency: "usd"})

	uc := NewUpdateMovement(repo, accounts, &fakeSyncTrigger{})

	cases := []struct {
		name  string
		input UpdateMovementInput
	}{
		{"zero amount", UpdateMovementInput{Amount: int64Ptr(0)}},
		{"unknown category", UpdateMovementInput{Category: (*entities.Category)(strPtr("yacht"))}},
		{"unknown payment method", UpdateMovementInput{PaymentMethod: (*entities.PaymentMethod)(strPtr("iou"))}},
		{"account belongs to another user", UpdateMovementInput{AccountID: &account.ID}},
		{"account currency mismatch", UpdateMovementInput{AccountID: func() *string {
			mismatched, _ := accounts.Create(context.Background(), &entities.Account{UserID: "u1", Currency: "brl"})
			return &mismatched.ID
		}()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := uc.Execute(context.Background(), "m1", tc.input); !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Errorf("want ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestUpdateMovementClearsAccountWithEmptyString(t *testing.T) {
	repo := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	account, _ := accounts.Create(context.Background(), &entities.Account{UserID: "u1", Currency: "usd"})
	m := activeMovement("m1", -500, entities.SyncStatusSynced)
	m.AccountID = &account.ID
	repo.add(m)

	uc := NewUpdateMovement(repo, accounts, &fakeSyncTrigger{})
	empty := ""
	result, err := uc.Execute(context.Background(), "m1", UpdateMovementInput{AccountID: &empty})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Movement.AccountID != nil {
		t.Errorf("account not cleared: %+v", result.Movement.AccountID)
	}
}

func TestUpdateMovementMissingMovement(t *testing.T) {
	repo := newFakeMovementRepo()
	uc := NewUpdateMovement(repo, newFakeAccountRepo(), &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), "nope", UpdateMovementInput{Description: strPtr("x")}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
