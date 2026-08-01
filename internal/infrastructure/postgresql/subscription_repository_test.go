package postgresql

import (
	"context"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestSubscriptionGetByUserIDNotFoundWhenNoRow(t *testing.T) {
	repo := NewSubscriptionRepository(openTestDB(t))
	if _, err := repo.GetByUserID(context.Background(), "no-such-user"); !apperrors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestSubscriptionUpsertRoundtripAndReplace(t *testing.T) {
	repo := NewSubscriptionRepository(openTestDB(t))
	ctx := context.Background()
	userID := "00000000-0000-0000-0000-000000000001"
	periodEnd := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	created, err := repo.Upsert(ctx, &dto.SubscriptionDTO{
		UserID: userID, Provider: "stripe", ProviderSubscriptionID: "sub_1",
		Status: dto.SubscriptionStatusActive, CurrentPeriodEnd: periodEnd,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != dto.SubscriptionStatusActive || !created.CurrentPeriodEnd.Equal(periodEnd) {
		t.Errorf("unexpected roundtrip: %+v", created)
	}

	newPeriodEnd := periodEnd.AddDate(1, 0, 0)
	updated, err := repo.Upsert(ctx, &dto.SubscriptionDTO{
		UserID: userID, Provider: "stripe", ProviderSubscriptionID: "sub_2",
		Status: dto.SubscriptionStatusPastDue, CurrentPeriodEnd: newPeriodEnd,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != dto.SubscriptionStatusPastDue || updated.ProviderSubscriptionID != "sub_2" {
		t.Errorf("upsert should replace the row in place, got %+v", updated)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt should be preserved across an upsert, got %v want %v", updated.CreatedAt, created.CreatedAt)
	}

	got, err := repo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProviderSubscriptionID != "sub_2" {
		t.Errorf("GetByUserID should reflect the latest upsert, got %+v", got)
	}
}

func TestSubscriptionListLapsable(t *testing.T) {
	repo := NewSubscriptionRepository(openTestDB(t))
	ctx := context.Background()
	now := time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)

	if _, err := repo.Upsert(ctx, &dto.SubscriptionDTO{
		UserID: "lapsable", Provider: "stripe", ProviderSubscriptionID: "sub_lapsable",
		Status: dto.SubscriptionStatusPastDue, CurrentPeriodEnd: now.AddDate(0, 0, -10),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Upsert(ctx, &dto.SubscriptionDTO{
		UserID: "still-in-grace", Provider: "stripe", ProviderSubscriptionID: "sub_grace",
		Status: dto.SubscriptionStatusPastDue, CurrentPeriodEnd: now.AddDate(0, 0, -2),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Upsert(ctx, &dto.SubscriptionDTO{
		UserID: "active", Provider: "stripe", ProviderSubscriptionID: "sub_active",
		Status: dto.SubscriptionStatusActive, CurrentPeriodEnd: now.AddDate(0, 0, -30),
	}); err != nil {
		t.Fatal(err)
	}
	// Canceled, period ended 10 days ago: past the grace period too — an
	// explicit cancellation gets the same leniency as a late payment.
	if _, err := repo.Upsert(ctx, &dto.SubscriptionDTO{
		UserID: "canceled-lapsable", Provider: "stripe", ProviderSubscriptionID: "sub_canceled",
		Status: dto.SubscriptionStatusCanceled, CurrentPeriodEnd: now.AddDate(0, 0, -10),
	}); err != nil {
		t.Fatal(err)
	}

	lapsable, err := repo.ListLapsable(ctx, now, 7)
	if err != nil {
		t.Fatal(err)
	}
	gotUserIDs := map[string]bool{}
	for _, s := range lapsable {
		gotUserIDs[s.UserID] = true
	}
	if len(lapsable) != 2 || !gotUserIDs["lapsable"] || !gotUserIDs["canceled-lapsable"] {
		t.Fatalf("ListLapsable = %+v, want exactly [lapsable, canceled-lapsable]", lapsable)
	}
}
