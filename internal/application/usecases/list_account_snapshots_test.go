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

func TestListAccountSnapshotsComputesReturnPerEntry(t *testing.T) {
	accounts := newFakeAccountRepo()
	movements := newFakeMovementRepo()
	ctx := context.Background()

	acc, _ := accounts.Create(ctx, &dto.AccountDTO{UserID: "u1", Name: "Checking", Currency: "usd"})

	day1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	// A movement between account creation and the first snapshot, and
	// another between the first and second snapshot.
	m1 := activeMovement("", -100, entities.SyncStatusPending)
	m1.AccountID = &acc.ID
	m1.Timestamp = day1.AddDate(0, 0, -1)
	movements.Create(ctx, m1)

	m2 := activeMovement("", -50, entities.SyncStatusPending)
	m2.AccountID = &acc.ID
	m2.Timestamp = day1.AddDate(0, 0, 15)
	movements.Create(ctx, m2)

	// First report: balance 1000, tracked movements say -100 since
	// account creation (implicit zero baseline) -> return = 1000 - 0 - (-100) = 1100.
	accounts.AddSnapshot(ctx, &dto.AccountSnapshotDTO{AccountID: acc.ID, Balance: 1000, Timestamp: day1, CreatedAt: day1})
	// Second report: balance 2000, tracked movements say -50 since the
	// first snapshot -> return = 2000 - 1000 - (-50) = 1050.
	accounts.AddSnapshot(ctx, &dto.AccountSnapshotDTO{AccountID: acc.ID, Balance: 2000, Timestamp: day2, CreatedAt: day2})

	uc := NewListAccountSnapshots(accounts, movements)
	views, err := uc.Execute(ctx, acc.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("got %d snapshots, want 2", len(views))
	}

	// Newest first.
	if views[0].Snapshot.Balance != 2000 || views[0].Return != 1050 {
		t.Errorf("newest snapshot = %+v, want balance=2000 return=1050", views[0])
	}
	if views[0].ReturnFrom == nil || !views[0].ReturnFrom.Equal(day1) {
		t.Errorf("newest snapshot return_from = %v, want %v", views[0].ReturnFrom, day1)
	}

	if views[1].Snapshot.Balance != 1000 || views[1].Return != 1100 {
		t.Errorf("oldest snapshot = %+v, want balance=1000 return=1100", views[1])
	}
	if views[1].ReturnFrom != nil {
		t.Errorf("oldest snapshot should have no return_from, got %v", views[1].ReturnFrom)
	}
}

func TestListAccountSnapshotsRejectsUnknownAccount(t *testing.T) {
	uc := NewListAccountSnapshots(newFakeAccountRepo(), newFakeMovementRepo())
	if _, err := uc.Execute(context.Background(), "missing"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestListAccountSnapshotsEmptyHistory(t *testing.T) {
	accounts := newFakeAccountRepo()
	acc, _ := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "u1", Name: "Checking", Currency: "usd"})

	uc := NewListAccountSnapshots(accounts, newFakeMovementRepo())
	views, err := uc.Execute(context.Background(), acc.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(views) != 0 {
		t.Errorf("got %d snapshots, want 0 for an account never reported on", len(views))
	}
}
