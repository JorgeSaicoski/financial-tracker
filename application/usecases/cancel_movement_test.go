package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

func activeMovement(id string, amount int64, syncStatus entities.SyncStatus) *entities.Movement {
	return &entities.Movement{
		ID:            id,
		UserID:        "u1",
		Amount:        amount,
		Currency:      "usd",
		Category:      entities.CategoryFood,
		PaymentMethod: entities.PaymentMethodPix,
		Status:        entities.MovementStatusActive,
		SyncStatus:    syncStatus,
		Timestamp:     time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
	}
}

func TestCancelUnsyncedMovementVoidsLocally(t *testing.T) {
	repo := newFakeMovementRepo()
	trigger := &fakeSyncTrigger{}
	repo.add(activeMovement("m1", -500, entities.SyncStatusPending))

	result, err := NewCancelMovement(repo, trigger).Execute(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reversal != nil {
		t.Error("unsynced cancel must not create a reversal")
	}
	if result.Movement.Status != entities.MovementStatusVoided {
		t.Errorf("status = %q, want voided", result.Movement.Status)
	}
	if trigger.calls != 0 {
		t.Error("voiding locally must not trigger a sync")
	}

	// The sync worker must never pick a voided movement up.
	pending, _ := repo.ListPendingSync(context.Background(), time.Now().UTC().Add(time.Hour), 0)
	if len(pending) != 0 {
		t.Errorf("voided movement still pending sync: %v", pending)
	}
}

func TestCancelFailedSyncMovementVoidsLocally(t *testing.T) {
	// "failed" means it never reached ledger-service either — void it.
	repo := newFakeMovementRepo()
	repo.add(activeMovement("m1", -500, entities.SyncStatusFailed))

	result, err := NewCancelMovement(repo, &fakeSyncTrigger{}).Execute(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reversal != nil || result.Movement.Status != entities.MovementStatusVoided {
		t.Errorf("failed-sync movement should void, got %+v", result)
	}
}

func TestCancelSyncedMovementCreatesReversal(t *testing.T) {
	repo := newFakeMovementRepo()
	trigger := &fakeSyncTrigger{}
	original := activeMovement("m1", -500, entities.SyncStatusSynced)
	repo.add(original)

	result, err := NewCancelMovement(repo, trigger).Execute(context.Background(), "m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rev := result.Reversal
	if rev == nil {
		t.Fatal("synced cancel must create a reversal")
	}
	if rev.Amount != 500 {
		t.Errorf("reversal amount = %d, want 500", rev.Amount)
	}
	if rev.CancelsMovementID == nil || *rev.CancelsMovementID != "m1" {
		t.Error("reversal not linked to original")
	}
	if rev.SyncStatus != entities.SyncStatusPending || rev.Status != entities.MovementStatusActive {
		t.Errorf("reversal state = %s/%s, want active/pending", rev.Status, rev.SyncStatus)
	}
	if rev.Category != original.Category || rev.PaymentMethod != original.PaymentMethod {
		t.Error("reversal should copy category and payment method")
	}

	// Original stays active (immutable once in ledger-service) but is
	// marked reversed.
	stored, _ := repo.GetByID(context.Background(), "m1")
	if stored.Status != entities.MovementStatusActive {
		t.Errorf("original status = %q, want active", stored.Status)
	}
	if stored.ReversedByMovementID == nil || *stored.ReversedByMovementID != rev.ID {
		t.Error("original not linked to reversal")
	}
	if trigger.calls != 1 {
		t.Errorf("sync trigger calls = %d, want 1", trigger.calls)
	}
}

func TestCancelRejectsBadStates(t *testing.T) {
	repo := newFakeMovementRepo()
	uc := NewCancelMovement(repo, &fakeSyncTrigger{})

	voided := activeMovement("voided", -100, entities.SyncStatusPending)
	voided.Status = entities.MovementStatusVoided
	repo.add(voided)

	reversedID := "rev"
	reversed := activeMovement("reversed", -100, entities.SyncStatusSynced)
	reversed.ReversedByMovementID = &reversedID
	repo.add(reversed)

	originalID := "reversed"
	reversal := activeMovement("rev", 100, entities.SyncStatusPending)
	reversal.CancelsMovementID = &originalID
	repo.add(reversal)

	cases := []struct {
		name string
		id   string
		want error
	}{
		{"missing movement", "nope", apperrors.ErrNotFound},
		{"already voided", "voided", apperrors.ErrConflict},
		{"already reversed", "reversed", apperrors.ErrConflict},
		{"a reversal itself", "rev", apperrors.ErrInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := uc.Execute(context.Background(), tc.id); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
}

func TestCancelledMovementsNetToZeroInBalance(t *testing.T) {
	repo := newFakeMovementRepo()
	trigger := &fakeSyncTrigger{}
	cancel := NewCancelMovement(repo, trigger)
	list := NewListMovements(repo)

	repo.add(activeMovement("kept", -250, entities.SyncStatusSynced))
	repo.add(activeMovement("synced-cancelled", -500, entities.SyncStatusSynced))
	repo.add(activeMovement("unsynced-cancelled", -900, entities.SyncStatusPending))

	if _, err := cancel.Execute(context.Background(), "synced-cancelled"); err != nil {
		t.Fatal(err)
	}
	if _, err := cancel.Execute(context.Background(), "unsynced-cancelled"); err != nil {
		t.Fatal(err)
	}

	result, err := list.Execute(context.Background(), "u1", nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Only "kept" should remain in the balance: the synced-cancelled
	// original and its reversal net out; the voided one is excluded.
	if result.Balance != -250 {
		t.Errorf("balance = %d, want -250", result.Balance)
	}
}
