package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func strPtr(s string) *string { return &s }
func int64Ptr(n int64) *int64 { return &n }

func TestUpdateMovementMetadataOnSyncedMovementEditsInPlace(t *testing.T) {
	repo := newFakeMovementRepo()
	trigger := &fakeSyncTrigger{}
	repo.add(activeMovement("m1", -500, entities.SyncStatusSynced))
	categories := newFakeCategoryRepo()
	transport, err := categories.Create(context.Background(), &dto.CategoryDTO{Name: "transport", ContributorIDs: []string{"u1"}})
	if err != nil {
		t.Fatal(err)
	}

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), categories, trigger)
	newDescription := "corrected description"
	result, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{
		Description: &newDescription,
		CategoryID:  &transport.ID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reversal != nil || result.Replacement != nil {
		t.Errorf("metadata-only edit must not reverse/recreate: %+v", result)
	}
	if result.Movement.Description != newDescription || result.Movement.CategoryID == nil || *result.Movement.CategoryID != transport.ID {
		t.Errorf("metadata not applied: %+v", result.Movement)
	}
	if result.Movement.Amount != -500 {
		t.Errorf("amount changed unexpectedly: %d", result.Movement.Amount)
	}
	if trigger.calls != 0 {
		t.Error("metadata-only edit must not trigger a sync")
	}

	stored, _ := repo.GetByID(context.Background(), "m1")
	if stored.Description != newDescription || stored.CategoryID == nil || *stored.CategoryID != transport.ID {
		t.Errorf("metadata not persisted: %+v", stored)
	}
}

func TestUpdateMovementAmountPreSyncEditsInPlace(t *testing.T) {
	repo := newFakeMovementRepo()
	trigger := &fakeSyncTrigger{}
	repo.add(activeMovement("m1", -500, entities.SyncStatusPending))

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), trigger)
	result, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{
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

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), trigger)
	newDescription := "corrected"
	_, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{
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

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), trigger)
	result, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{
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
	if result.Replacement.SyncStatus != string(entities.SyncStatusPending) {
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

// TestUpdateMovementPostSyncRollsBackReversalWhenReplacementCreationFails
// is the regression test for a real bug: a synced movement's amount edit
// used to cancel (reverse) the original in one write, then create the
// replacement in a second, unrelated write. If that second write ever
// failed — a transient error, a full disk, anything — the reversal had
// already committed, so the movement was left reversed with no
// replacement: money silently vanished. The fix wraps both writes in one
// Transact; this test forces the second write to fail and asserts the
// first rolls back with it, so a synced movement's edit is all-or-nothing.
func TestUpdateMovementPostSyncRollsBackReversalWhenReplacementCreationFails(t *testing.T) {
	repo := newFakeMovementRepo()
	trigger := &fakeSyncTrigger{}
	repo.add(activeMovement("m1", 10000, entities.SyncStatusSynced))
	repo.createErr = errors.New("simulated write failure creating the replacement")

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), trigger)
	_, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{
		Amount: int64Ptr(20000),
	})
	if !errors.Is(err, repo.createErr) {
		t.Fatalf("want the create error surfaced, got %v", err)
	}
	if trigger.calls != 0 {
		t.Error("a failed update must not trigger a sync")
	}

	original, err := repo.GetByID(context.Background(), "m1")
	if err != nil {
		t.Fatal(err)
	}
	if original.ReversedByMovementID != nil {
		t.Errorf("original must NOT be left reversed when the replacement never got created: %+v", original)
	}
	if original.Amount != 10000 {
		t.Errorf("original amount must be untouched, got %d", original.Amount)
	}

	all, err := repo.ListByUser(context.Background(), "u1", nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("a dangling reversal (or anything else) must not have survived: got %d movements: %+v", len(all), all)
	}
}

// TestUpdateMovementPostSyncCombinedAmountAndMetadataEdit covers editing a
// synced movement's amount and metadata together: the replacement must
// carry the new metadata (not the original's), and the original's own
// metadata must stay exactly as it was — it's a historical record now.
func TestUpdateMovementPostSyncCombinedAmountAndMetadataEdit(t *testing.T) {
	repo := newFakeMovementRepo()
	categories := newFakeCategoryRepo()
	other, err := categories.Create(context.Background(), &dto.CategoryDTO{ID: entities.CategoryOtherID, Name: entities.CategoryOther})
	if err != nil {
		t.Fatal(err)
	}
	income, err := categories.Create(context.Background(), &dto.CategoryDTO{ID: entities.CategoryIncomeID, Name: entities.CategoryIncome})
	if err != nil {
		t.Fatal(err)
	}
	m := activeMovement("m1", 10000, entities.SyncStatusSynced)
	m.Description = "old description"
	m.CategoryID = &other.ID
	repo.add(m)

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), categories, &fakeSyncTrigger{})
	newDescription := "salary, corrected"
	result, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{
		Amount:      int64Ptr(20000),
		Description: &newDescription,
		CategoryID:  &income.ID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Replacement == nil {
		t.Fatal("expected a replacement")
	}
	if result.Replacement.Amount != 20000 || result.Replacement.Description != newDescription ||
		result.Replacement.CategoryID == nil || *result.Replacement.CategoryID != income.ID {
		t.Errorf("replacement must carry the new amount and metadata: %+v", result.Replacement)
	}

	original, _ := repo.GetByID(context.Background(), "m1")
	if original.Description != "old description" || original.CategoryID == nil || *original.CategoryID != other.ID {
		t.Errorf("original's metadata must stay exactly as it was, it's a historical record: %+v", original)
	}
}

// TestUpdateMovementCurrencyOnlyPostSyncReversesAndRecreates covers editing
// just the currency (no amount change) on an already-synced movement —
// editsFinancial must fire for currency alone, same as for amount.
func TestUpdateMovementCurrencyOnlyPostSyncReversesAndRecreates(t *testing.T) {
	repo := newFakeMovementRepo()
	repo.add(activeMovement("m1", 10000, entities.SyncStatusSynced))

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), &fakeSyncTrigger{})
	newCurrency := "brl"
	result, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{
		Currency: &newCurrency,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reversal == nil || result.Replacement == nil {
		t.Fatalf("currency-only edit on a synced movement must reverse + recreate: %+v", result)
	}
	if result.Replacement.Currency != "brl" || result.Replacement.Amount != 10000 {
		t.Errorf("replacement = %+v, want amount unchanged and currency brl", result.Replacement)
	}
}

func TestUpdateMovementRejectsVoidedMovement(t *testing.T) {
	repo := newFakeMovementRepo()
	voided := activeMovement("m1", -500, entities.SyncStatusPending)
	voided.Status = string(entities.MovementStatusVoided)
	repo.add(voided)

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{Amount: int64Ptr(-1)}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestUpdateMovementRejectsReversedMovement(t *testing.T) {
	repo := newFakeMovementRepo()
	reversedID := "rev"
	reversed := activeMovement("m1", -500, entities.SyncStatusSynced)
	reversed.ReversedByMovementID = &reversedID
	repo.add(reversed)

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{Amount: int64Ptr(-1)}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestUpdateMovementRejectsReversalItself(t *testing.T) {
	repo := newFakeMovementRepo()
	originalID := "original"
	reversal := activeMovement("rev", 500, entities.SyncStatusPending)
	reversal.CancelsMovementID = &originalID
	repo.add(reversal)

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), &fakeSyncTrigger{})
	newDescription := "nope"
	if _, err := uc.Execute(context.Background(), "u1", "rev", UpdateMovementInput{Description: &newDescription}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestUpdateMovementRejectsInstallmentFinancialEdit(t *testing.T) {
	repo := newFakeMovementRepo()
	purchaseID := "p1"
	installment := activeMovement("m1", -300, entities.SyncStatusPending)
	installment.CreditCardPurchaseID = &purchaseID
	repo.add(installment)

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{Amount: int64Ptr(-400)}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}

	// Metadata edits on installments are still fine.
	newDescription := "renamed"
	result, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{Description: &newDescription})
	if err != nil {
		t.Fatalf("metadata edit on installment should succeed: %v", err)
	}
	if result.Movement.Description != newDescription {
		t.Errorf("description not applied: %+v", result.Movement)
	}
}

func TestUpdateMovementRejectsTransferFinancialEdit(t *testing.T) {
	repo := newFakeMovementRepo()
	m := activeMovement("m1", -500, entities.SyncStatusPending)
	transferID := "t1"
	m.TransferID = &transferID
	repo.add(m)

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{Amount: int64Ptr(-1)}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestUpdateMovementValidatesLikeCreate(t *testing.T) {
	repo := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	repo.add(activeMovement("m1", -500, entities.SyncStatusPending))
	account, _ := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "someone-else", Currency: "usd"})

	uc := NewUpdateMovement(repo, accounts, newFakePaymentMethodRepo(), newFakeCategoryRepo(), &fakeSyncTrigger{})

	cases := []struct {
		name  string
		input UpdateMovementInput
	}{
		{"zero amount", UpdateMovementInput{Amount: int64Ptr(0)}},
		// "unknown category" and "unknown payment method" are intentionally
		// absent: CategoryID is FK-based now (resolveCategoryID's rejection
		// of an unowned/unknown id is covered by the
		// TestCreateMovementRejects* tests) and an unrecognized payment
		// method is implicitly registered, not rejected (BACK-17).
		{"avoidability_percent out of range", UpdateMovementInput{AvoidabilityOverridePercent: intPtrAv(-1)}},
		{"account belongs to another user", UpdateMovementInput{AccountID: &account.ID}},
		{"account currency mismatch", UpdateMovementInput{AccountID: func() *string {
			mismatched, _ := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "u1", Currency: "brl"})
			return &mismatched.ID
		}()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := uc.Execute(context.Background(), "u1", "m1", tc.input); !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Errorf("want ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestUpdateMovementClearsAccountWithEmptyString(t *testing.T) {
	repo := newFakeMovementRepo()
	accounts := newFakeAccountRepo()
	account, _ := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "u1", Currency: "usd"})
	m := activeMovement("m1", -500, entities.SyncStatusSynced)
	m.AccountID = &account.ID
	repo.add(m)

	uc := NewUpdateMovement(repo, accounts, newFakePaymentMethodRepo(), newFakeCategoryRepo(), &fakeSyncTrigger{})
	empty := ""
	result, err := uc.Execute(context.Background(), "u1", "m1", UpdateMovementInput{AccountID: &empty})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Movement.AccountID != nil {
		t.Errorf("account not cleared: %+v", result.Movement.AccountID)
	}
}

func TestUpdateMovementMissingMovement(t *testing.T) {
	repo := newFakeMovementRepo()
	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), "u1", "nope", UpdateMovementInput{Description: strPtr("x")}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestUpdateMovementRejectsCrossUserAccess(t *testing.T) {
	repo := newFakeMovementRepo()
	repo.add(activeMovement("m1", -500, entities.SyncStatusPending)) // owned by "u1"

	uc := NewUpdateMovement(repo, newFakeAccountRepo(), newFakePaymentMethodRepo(), newFakeCategoryRepo(), &fakeSyncTrigger{})
	if _, err := uc.Execute(context.Background(), "someone-else", "m1", UpdateMovementInput{Description: strPtr("x")}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("want ErrNotFound for another user's movement, got %v", err)
	}
}
