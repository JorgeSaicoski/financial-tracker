package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func testExchangeRate(currency, unitsPerUSD string, effectiveFrom time.Time) *dto.ExchangeRateDTO {
	return &dto.ExchangeRateDTO{
		UserID:        "00000000-0000-0000-0000-000000000001",
		Currency:      currency,
		UnitsPerUSD:   unitsPerUSD,
		EffectiveFrom: effectiveFrom,
		CreatedAt:     time.Now().UTC(),
	}
}

func TestExchangeRateCreateUpsertsOnSameDate(t *testing.T) {
	repo := NewExchangeRateRepository(openTestDB(t))
	ctx := context.Background()
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	created, err := repo.Create(ctx, testExchangeRate("brl", "5", day))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("no id generated")
	}

	// Same (user, currency, effective_from) again must replace the rate
	// in place — same id, same original created_at — not add a row.
	updated, err := repo.Create(ctx, testExchangeRate("brl", "5.5", day))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if updated.ID != created.ID {
		t.Errorf("upsert changed id: got %s, want %s", updated.ID, created.ID)
	}

	all, err := repo.ListByUser(ctx, "00000000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("expected exactly 1 row after upsert, got %d", len(all))
	}
	if all[0].UnitsPerUSD != "5.5" {
		t.Errorf("units_per_usd = %s, want 5.5 (replaced)", all[0].UnitsPerUSD)
	}
}

func TestExchangeRateAtPicksEffectiveRow(t *testing.T) {
	repo := NewExchangeRateRepository(openTestDB(t))
	ctx := context.Background()
	userID := "00000000-0000-0000-0000-000000000001"

	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	feb1 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	mar1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	if _, err := repo.Create(ctx, testExchangeRate("brl", "5", jan1)); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, testExchangeRate("brl", "6", feb1)); err != nil {
		t.Fatal(err)
	}

	// Before any rate exists.
	if _, err := repo.RateAt(ctx, userID, "brl", jan1.AddDate(0, 0, -1)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("before first rate: want ErrNotFound, got %v", err)
	}

	// Exactly on jan1 gets the jan1 rate (greatest EffectiveFrom <= at).
	rate, err := repo.RateAt(ctx, userID, "brl", jan1)
	if err != nil {
		t.Fatal(err)
	}
	if rate.UnitsPerUSD != "5" {
		t.Errorf("at jan1: got %s, want 5", rate.UnitsPerUSD)
	}

	// Between jan1 and feb1 still gets the jan1 rate.
	rate, err = repo.RateAt(ctx, userID, "brl", jan1.AddDate(0, 0, 15))
	if err != nil {
		t.Fatal(err)
	}
	if rate.UnitsPerUSD != "5" {
		t.Errorf("mid-january: got %s, want 5", rate.UnitsPerUSD)
	}

	// At or after feb1 gets the feb1 rate.
	rate, err = repo.RateAt(ctx, userID, "brl", mar1)
	if err != nil {
		t.Fatal(err)
	}
	if rate.UnitsPerUSD != "6" {
		t.Errorf("at mar1: got %s, want 6 (most recent rate)", rate.UnitsPerUSD)
	}

	// A different currency with no rows of its own is not found.
	if _, err := repo.RateAt(ctx, userID, "eur", mar1); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("currency with no rates: want ErrNotFound, got %v", err)
	}
}

func TestExchangeRateDelete(t *testing.T) {
	repo := NewExchangeRateRepository(openTestDB(t))
	ctx := context.Background()
	userID := "00000000-0000-0000-0000-000000000001"
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	created, err := repo.Create(ctx, testExchangeRate("brl", "5", day))
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, userID, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	all, err := repo.ListByUser(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 rows after delete, got %d", len(all))
	}

	// Deleting again (already gone) is a not-found, not a silent no-op.
	if err := repo.Delete(ctx, userID, created.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("delete missing row: want ErrNotFound, got %v", err)
	}

	// Deleting a row owned by a different user must not succeed —
	// reference data is user-owned, not globally shared.
	other, err := repo.Create(ctx, testExchangeRate("brl", "5", day))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, "00000000-0000-0000-0000-000000000002", other.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("delete another user's row: want ErrNotFound, got %v", err)
	}
}
