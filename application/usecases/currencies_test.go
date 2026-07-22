package usecases

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

func TestAddCurrencyNormalizesAndValidates(t *testing.T) {
	uc := NewAddCurrency(newFakeCurrencyRepo())

	code, err := uc.Execute(context.Background(), "  BTC  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "btc" {
		t.Errorf("code = %q, want lowercased/trimmed %q", code, "btc")
	}
}

func TestAddCurrencyRejectsBadCodes(t *testing.T) {
	uc := NewAddCurrency(newFakeCurrencyRepo())

	for _, bad := range []string{"", "x", "this-is-way-too-long", "USD!"} {
		if _, err := uc.Execute(context.Background(), bad); !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Errorf("code %q: want ErrInvalidInput, got %v", bad, err)
		}
	}
}

func TestAddCurrencyIsIdempotent(t *testing.T) {
	repo := newFakeCurrencyRepo("usd")
	uc := NewAddCurrency(repo)

	if _, err := uc.Execute(context.Background(), "usd"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	list := NewListCurrencies(repo)
	codes, err := list.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(codes) != 1 || codes[0] != "usd" {
		t.Errorf("codes = %v, want exactly [usd]", codes)
	}
}

func TestListCurrencies(t *testing.T) {
	repo := newFakeCurrencyRepo("usd", "brl")
	uc := NewListCurrencies(repo)

	codes, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(codes) != 2 {
		t.Errorf("codes = %v, want 2 entries", codes)
	}
}
