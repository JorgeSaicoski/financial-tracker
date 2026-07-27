package crypto

import (
	"context"
	"testing"

	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
)

func TestPseudonymForIsStableAndNeverTheRealUserID(t *testing.T) {
	p := NewLedgerPseudonymizer(testKey(), newFakeLedgerPseudonymRepo())
	ctx := context.Background()
	realUserID := "11111111-1111-1111-1111-111111111111"

	first, err := p.PseudonymFor(ctx, realUserID)
	if err != nil {
		t.Fatal(err)
	}
	if first == realUserID {
		t.Fatal("pseudonym must never equal the real user id")
	}
	if !id.IsUUID(first) {
		t.Errorf("pseudonym %q is not a canonical UUID", first)
	}

	second, err := p.PseudonymFor(ctx, realUserID)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Errorf("PseudonymFor must return the same UUID on every call, got %q then %q", first, second)
	}
}

func TestPseudonymForGeneratesExactlyOnePerUser(t *testing.T) {
	repo := newFakeLedgerPseudonymRepo()
	p := NewLedgerPseudonymizer(testKey(), repo)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := p.PseudonymFor(ctx, "user-1"); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.rows) != 1 {
		t.Fatalf("want exactly one pseudonym row for user-1, got %d", len(repo.rows))
	}
}

func TestPseudonymForDiffersAcrossUsers(t *testing.T) {
	p := NewLedgerPseudonymizer(testKey(), newFakeLedgerPseudonymRepo())
	ctx := context.Background()

	a, err := p.PseudonymFor(ctx, "user-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.PseudonymFor(ctx, "user-b")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("different users must get different pseudonyms")
	}
}

func TestTokenizeCurrencyIsDeterministicAndDistinct(t *testing.T) {
	p := NewLedgerPseudonymizer(testKey(), newFakeLedgerPseudonymRepo())
	ctx := context.Background()

	usd1, err := p.TokenizeCurrency(ctx, "user-1", "usd")
	if err != nil {
		t.Fatal(err)
	}
	usd2, err := p.TokenizeCurrency(ctx, "user-1", "usd")
	if err != nil {
		t.Fatal(err)
	}
	if usd1 != usd2 {
		t.Errorf("same (user, currency) must tokenize identically: %q != %q", usd1, usd2)
	}

	brl, err := p.TokenizeCurrency(ctx, "user-1", "brl")
	if err != nil {
		t.Fatal(err)
	}
	if brl == usd1 {
		t.Error("different currencies for the same user must tokenize distinctly")
	}

	otherUserUSD, err := p.TokenizeCurrency(ctx, "user-2", "usd")
	if err != nil {
		t.Fatal(err)
	}
	if otherUserUSD == usd1 {
		t.Error("same currency for different users must tokenize distinctly")
	}

	if usd1 == "usd" {
		t.Error("token must not equal the plain currency code")
	}
}
