package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestImportArchiveRestoresEverything(t *testing.T) {
	ctx := context.Background()
	accounts := newFakeAccountRepo()
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)
	uc := NewImportArchive(accounts, movements, purchases)

	bundle := ArchiveBundle{
		Accounts: []*dto.AccountDTO{
			{ID: "acc-1", UserID: "user-1", Name: "Checking", Type: "bank", Currency: "usd"},
		},
		CreditCardPurchases: []*dto.CreditCardPurchaseDTO{
			{ID: "purchase-1", UserID: "user-1", Category: "shopping", TotalAmount: -900, Currency: "usd", InstallmentCount: 1, Status: string(entities.CreditCardPurchaseStatusActive)},
		},
		Movements: []*dto.MovementDTO{
			func() *dto.MovementDTO {
				m := activeMovement("mov-1", -450, entities.SyncStatusPending)
				m.UserID = "user-1"
				return m
			}(),
		},
	}

	result, err := uc.Execute(ctx, "user-1", bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccountsRestored != 1 || result.MovementsRestored != 1 || result.CreditCardPurchasesRestored != 1 {
		t.Errorf("result = %+v, want 1 restored of each", result)
	}

	if _, err := accounts.GetByID(ctx, "acc-1"); err != nil {
		t.Errorf("account not restored: %v", err)
	}
	if _, err := purchases.GetByID(ctx, "purchase-1"); err != nil {
		t.Errorf("purchase not restored: %v", err)
	}
	got, err := movements.GetByID(ctx, "mov-1")
	if err != nil {
		t.Fatalf("movement not restored: %v", err)
	}
	if got.Amount != -450 {
		t.Errorf("movement amount = %d, want -450", got.Amount)
	}
}

func TestImportArchiveIsIdempotent(t *testing.T) {
	ctx := context.Background()
	accounts := newFakeAccountRepo()
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)
	uc := NewImportArchive(accounts, movements, purchases)

	m := activeMovement("mov-1", -100, entities.SyncStatusPending)
	bundle := ArchiveBundle{
		Accounts:  []*dto.AccountDTO{{ID: "acc-1", UserID: "user-1", Name: "Checking", Type: "bank", Currency: "usd"}},
		Movements: []*dto.MovementDTO{m},
	}

	if _, err := uc.Execute(ctx, "user-1", bundle); err != nil {
		t.Fatalf("first import: %v", err)
	}

	result, err := uc.Execute(ctx, "user-1", bundle)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if result.AccountsSkipped != 1 || result.MovementsSkipped != 1 {
		t.Errorf("second import result = %+v, want everything skipped", result)
	}
	if result.AccountsRestored != 0 || result.MovementsRestored != 0 {
		t.Errorf("second import result = %+v, want nothing re-restored", result)
	}
}

// TestImportArchiveDropsReversalLinks documents the known limitation: an
// original/reversal pair references each other via opposite-direction
// self-referencing foreign keys, so no single insertion order satisfies
// both without deferred FK checking (which this schema doesn't use).
// Restore keeps both movements' own data intact but clears the link
// fields rather than risk a broken restore.
func TestImportArchiveDropsReversalLinks(t *testing.T) {
	ctx := context.Background()
	accounts := newFakeAccountRepo()
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)
	uc := NewImportArchive(accounts, movements, purchases)

	original := activeMovement("mov-original", -100, entities.SyncStatusPending)
	reversalID := "mov-reversal"
	original.ReversedByMovementID = &reversalID

	reversal := activeMovement(reversalID, 100, entities.SyncStatusPending)
	originalID := "mov-original"
	reversal.CancelsMovementID = &originalID

	bundle := ArchiveBundle{Movements: []*dto.MovementDTO{original, reversal}}

	if _, err := uc.Execute(ctx, "user-1", bundle); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotOriginal, err := movements.GetByID(ctx, "mov-original")
	if err != nil {
		t.Fatalf("original not restored: %v", err)
	}
	if gotOriginal.ReversedByMovementID != nil {
		t.Error("expected ReversedByMovementID to be cleared on restore")
	}
	gotReversal, err := movements.GetByID(ctx, "mov-reversal")
	if err != nil {
		t.Fatalf("reversal not restored: %v", err)
	}
	if gotReversal.CancelsMovementID != nil {
		t.Error("expected CancelsMovementID to be cleared on restore")
	}
	if gotOriginal.Amount != -100 || gotReversal.Amount != 100 {
		t.Error("amounts should still restore correctly despite dropped links")
	}
}

// TestImportArchiveRejectsMovementReferencingUnownedAccount guards the
// same ownership boundary create_movement.go enforces on the normal write
// path (BACK-02): a hand-crafted archive body naming another user's real
// account id must not be able to attach a movement to it.
func TestImportArchiveRejectsMovementReferencingUnownedAccount(t *testing.T) {
	ctx := context.Background()
	accounts := newFakeAccountRepo()
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)
	uc := NewImportArchive(accounts, movements, purchases)

	other, err := accounts.Create(ctx, &dto.AccountDTO{UserID: "user-2", Name: "Victim's account", Type: "bank", Currency: "usd"})
	if err != nil {
		t.Fatal(err)
	}

	m := activeMovement("mov-1", -100, entities.SyncStatusPending)
	m.AccountID = &other.ID
	bundle := ArchiveBundle{Movements: []*dto.MovementDTO{m}}

	if _, err := uc.Execute(ctx, "user-1", bundle); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for a movement referencing another user's account, got %v", err)
	}
	if _, err := movements.GetByID(ctx, "mov-1"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Error("movement should not have been restored")
	}
}

// TestImportArchiveRejectsMovementReferencingUnownedPurchase mirrors the
// account case above for CreditCardPurchaseID.
func TestImportArchiveRejectsMovementReferencingUnownedPurchase(t *testing.T) {
	ctx := context.Background()
	accounts := newFakeAccountRepo()
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)
	uc := NewImportArchive(accounts, movements, purchases)

	other, _, err := purchases.CreateWithInstallments(ctx, &dto.CreditCardPurchaseDTO{
		UserID: "user-2", Category: "shopping", TotalAmount: -900,
		Currency: "usd", InstallmentCount: 1, Status: string(entities.CreditCardPurchaseStatusActive),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	m := activeMovement("mov-1", -100, entities.SyncStatusPending)
	m.CreditCardPurchaseID = &other.ID
	bundle := ArchiveBundle{Movements: []*dto.MovementDTO{m}}

	if _, err := uc.Execute(ctx, "user-1", bundle); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for a movement referencing another user's credit card purchase, got %v", err)
	}
}

// TestImportArchiveAllowsMovementReferencingAccountRestoredInSameBundle
// makes sure the ownership check doesn't reject the normal case: a
// movement referencing an account that's part of the same archive (not
// pre-existing) must still restore successfully.
func TestImportArchiveAllowsMovementReferencingAccountRestoredInSameBundle(t *testing.T) {
	ctx := context.Background()
	accounts := newFakeAccountRepo()
	movements := newFakeMovementRepo()
	purchases := newFakePurchaseRepo(movements)
	uc := NewImportArchive(accounts, movements, purchases)

	accountID := "acc-1"
	m := activeMovement("mov-1", -100, entities.SyncStatusPending)
	m.AccountID = &accountID
	bundle := ArchiveBundle{
		Accounts:  []*dto.AccountDTO{{ID: accountID, UserID: "user-1", Name: "Checking", Type: "bank", Currency: "usd"}},
		Movements: []*dto.MovementDTO{m},
	}

	result, err := uc.Execute(ctx, "user-1", bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.MovementsRestored != 1 {
		t.Errorf("result = %+v, want the movement restored", result)
	}
}

func TestImportArchiveRejectsEmptyUserID(t *testing.T) {
	uc := NewImportArchive(newFakeAccountRepo(), newFakeMovementRepo(), newFakePurchaseRepo(newFakeMovementRepo()))
	if _, err := uc.Execute(context.Background(), "", ArchiveBundle{}); err == nil {
		t.Error("expected an error for empty user id")
	}
}
