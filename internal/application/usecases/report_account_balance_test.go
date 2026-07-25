package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

func TestReportAccountBalanceDefaultsTimestampToNow(t *testing.T) {
	accounts := newFakeAccountRepo()
	acc, _ := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "u1", Name: "Checking", Currency: "usd"})

	uc := NewReportAccountBalance(accounts, newFakeMovementRepo())
	before := time.Now().UTC()
	view, err := uc.Execute(context.Background(), acc.ID, 1000, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.ReportedAt == nil || view.ReportedAt.Before(before.Add(-time.Second)) {
		t.Errorf("reported_at = %v, want close to now", view.ReportedAt)
	}
}

func TestReportAccountBalanceAcceptsExplicitTimestamp(t *testing.T) {
	accounts := newFakeAccountRepo()
	acc, _ := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "u1", Name: "Checking", Currency: "usd"})

	backdated := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	uc := NewReportAccountBalance(accounts, newFakeMovementRepo())
	view, err := uc.Execute(context.Background(), acc.ID, 500, backdated)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.ReportedAt == nil || !view.ReportedAt.Equal(backdated) {
		t.Errorf("reported_at = %v, want %v", view.ReportedAt, backdated)
	}
}
