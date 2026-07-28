package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestLedgerPseudonymGetReturnsNotFoundWhenAbsent(t *testing.T) {
	repo := NewLedgerPseudonymRepository(openTestDB(t))
	ctx := context.Background()

	if _, err := repo.Get(ctx, "no-such-user"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestLedgerPseudonymCreateAndGetRoundtrip(t *testing.T) {
	repo := NewLedgerPseudonymRepository(openTestDB(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, &dto.LedgerPseudonymDTO{
		UserID: "user-1", PseudonymID: "33333333-3333-3333-3333-333333333333", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.PseudonymID != "33333333-3333-3333-3333-333333333333" {
		t.Errorf("Create returned PseudonymID = %q", created.PseudonymID)
	}

	got, err := repo.Get(ctx, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.PseudonymID != "33333333-3333-3333-3333-333333333333" {
		t.Errorf("Get PseudonymID = %q", got.PseudonymID)
	}
}

func TestLedgerPseudonymCreateIsRaceSafe(t *testing.T) {
	repo := NewLedgerPseudonymRepository(openTestDB(t))
	ctx := context.Background()

	first, err := repo.Create(ctx, &dto.LedgerPseudonymDTO{
		UserID: "user-1", PseudonymID: "11111111-1111-1111-1111-111111111111", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	second, err := repo.Create(ctx, &dto.LedgerPseudonymDTO{
		UserID: "user-1", PseudonymID: "22222222-2222-2222-2222-222222222222", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.PseudonymID != first.PseudonymID {
		t.Errorf("second Create must return the existing pseudonym (%q), got %q", first.PseudonymID, second.PseudonymID)
	}
}
