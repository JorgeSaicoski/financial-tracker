package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

func mustDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ---- SetExchangeRateUseCase ----

func TestSetExchangeRateValidatesCurrencyAndRate(t *testing.T) {
	rates := newFakeExchangeRateRepo()
	currencies := newFakeCurrencyRepo("usd", "brl")
	uc := NewSetExchangeRate(rates, currencies)

	cases := []struct {
		name     string
		currency string
		units    string
	}{
		{"usd rejected", "usd", "1"},
		{"unregistered currency rejected", "eur", "5"},
		{"zero rate rejected", "brl", "0"},
		{"negative rate rejected", "brl", "-5"},
		{"non-numeric rate rejected", "brl", "not-a-number"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := uc.Execute(context.Background(), SetExchangeRateInput{
				UserID: "u1", Currency: tc.currency, UnitsPerUSD: tc.units,
			})
			if !errors.Is(err, apperrors.ErrInvalidInput) {
				t.Errorf("got %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestSetExchangeRateSameDateReplacesRow(t *testing.T) {
	rates := newFakeExchangeRateRepo()
	currencies := newFakeCurrencyRepo("usd", "brl")
	uc := NewSetExchangeRate(rates, currencies)
	ctx := context.Background()

	first, err := uc.Execute(ctx, SetExchangeRateInput{
		UserID: "u1", Currency: "brl", UnitsPerUSD: "5", EffectiveFrom: mustDate(2026, 1, 1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	second, err := uc.Execute(ctx, SetExchangeRateInput{
		UserID: "u1", Currency: "brl", UnitsPerUSD: "5.5", EffectiveFrom: mustDate(2026, 1, 1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if second.ID != first.ID {
		t.Errorf("backfill correction got a new row (id %q -> %q), want the same row replaced", first.ID, second.ID)
	}
	if second.UnitsPerUSD != "5.5" {
		t.Errorf("units_per_usd = %q, want %q", second.UnitsPerUSD, "5.5")
	}

	history, err := rates.ListByUser(ctx, "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("history has %d rows, want exactly 1 (replaced, not duplicated)", len(history))
	}
}

// ---- ToUSDUseCase: historical rate lookup ----

func TestToUSDPicksCorrectHistoricalRow(t *testing.T) {
	rates := newFakeExchangeRateRepo()
	currencies := newFakeCurrencyRepo("usd", "brl")
	setRate := NewSetExchangeRate(rates, currencies)
	toUSD := NewToUSD(rates, currencies)
	ctx := context.Background()

	if _, err := setRate.Execute(ctx, SetExchangeRateInput{
		UserID: "u1", Currency: "brl", UnitsPerUSD: "5", EffectiveFrom: mustDate(2026, 1, 1),
	}); err != nil {
		t.Fatalf("seed rate 1: %v", err)
	}
	if _, err := setRate.Execute(ctx, SetExchangeRateInput{
		UserID: "u1", Currency: "brl", UnitsPerUSD: "6", EffectiveFrom: mustDate(2026, 2, 1),
	}); err != nil {
		t.Fatalf("seed rate 2: %v", err)
	}

	t.Run("before first rate returns typed not-found error", func(t *testing.T) {
		_, err := toUSD.Execute(ctx, "u1", 1000, "brl", mustDate(2025, 12, 15))
		if !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("got %v, want ErrNotFound", err)
		}
	})

	t.Run("between two rates uses the earlier one", func(t *testing.T) {
		// 500 brl cents = 5.00 brl; at 5 brl/usd -> 1.00 usd -> 100 cents.
		got, err := toUSD.Execute(ctx, "u1", 500, "brl", mustDate(2026, 1, 15))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 100 {
			t.Errorf("got %d cents, want 100", got)
		}
	})

	t.Run("after latest rate uses the latest", func(t *testing.T) {
		// 600 brl cents = 6.00 brl; at 6 brl/usd -> 1.00 usd -> 100 cents.
		got, err := toUSD.Execute(ctx, "u1", 600, "brl", mustDate(2026, 3, 1))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 100 {
			t.Errorf("got %d cents, want 100", got)
		}
	})
}

func TestToUSDUpdatingTodaysRateDoesNotChangeLastMonthsConversions(t *testing.T) {
	rates := newFakeExchangeRateRepo()
	currencies := newFakeCurrencyRepo("usd", "brl")
	setRate := NewSetExchangeRate(rates, currencies)
	toUSD := NewToUSD(rates, currencies)
	ctx := context.Background()

	if _, err := setRate.Execute(ctx, SetExchangeRateInput{
		UserID: "u1", Currency: "brl", UnitsPerUSD: "5", EffectiveFrom: mustDate(2026, 1, 1),
	}); err != nil {
		t.Fatalf("seed last month's rate: %v", err)
	}

	lastMonthBefore, err := toUSD.Execute(ctx, "u1", 500, "brl", mustDate(2026, 1, 15))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// "Today's" rate: a new, later effective_from — not a correction of
	// January's row.
	if _, err := setRate.Execute(ctx, SetExchangeRateInput{
		UserID: "u1", Currency: "brl", UnitsPerUSD: "7", EffectiveFrom: mustDate(2026, 3, 1),
	}); err != nil {
		t.Fatalf("seed today's rate: %v", err)
	}

	lastMonthAfter, err := toUSD.Execute(ctx, "u1", 500, "brl", mustDate(2026, 1, 15))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lastMonthAfter != lastMonthBefore {
		t.Errorf("last month's conversion changed after posting today's rate: %d -> %d", lastMonthBefore, lastMonthAfter)
	}

	today, err := toUSD.Execute(ctx, "u1", 700, "brl", mustDate(2026, 3, 5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if today != 100 {
		t.Errorf("today's conversion = %d, want 100 (uses the new 7 brl/usd rate)", today)
	}
}

func TestToUSDRatesAreNotVisibleAcrossUsers(t *testing.T) {
	rates := newFakeExchangeRateRepo()
	currencies := newFakeCurrencyRepo("usd", "brl")
	setRate := NewSetExchangeRate(rates, currencies)
	toUSD := NewToUSD(rates, currencies)
	ctx := context.Background()

	if _, err := setRate.Execute(ctx, SetExchangeRateInput{
		UserID: "userA", Currency: "brl", UnitsPerUSD: "5", EffectiveFrom: mustDate(2026, 1, 1),
	}); err != nil {
		t.Fatalf("seed userA's rate: %v", err)
	}

	if _, err := toUSD.Execute(ctx, "userB", 500, "brl", mustDate(2026, 1, 15)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("userB got %v, want ErrNotFound (userA's rates must not leak across users)", err)
	}

	// Sanity: userA's own conversion still works.
	if _, err := toUSD.Execute(ctx, "userA", 500, "brl", mustDate(2026, 1, 15)); err != nil {
		t.Errorf("userA's own conversion failed: %v", err)
	}
}

func TestToUSDPassesUsdThrough(t *testing.T) {
	rates := newFakeExchangeRateRepo()
	currencies := newFakeCurrencyRepo("usd")
	toUSD := NewToUSD(rates, currencies)

	got, err := toUSD.Execute(context.Background(), "u1", 12345, "usd", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 12345 {
		t.Errorf("got %d, want 12345 unchanged (usd is the reference currency)", got)
	}
}

// ---- convertToUSDSmallestUnit: exact conversion math ----

func TestConvertToUSDSmallestUnitExactCases(t *testing.T) {
	cases := []struct {
		name             string
		amount           int64
		unitsPerUSD      string
		currencyDecimals int
		usdDecimals      int
		want             int64
	}{
		{"same decimals, whole rate", 500, "5", 2, 2, 100},          // 5.00 brl @ 5/usd = 1.00 usd = 100 cents
		{"fractional rate, exact division", 525, "5.25", 2, 2, 100}, // 5.25 brl @ 5.25/usd = 1.00 usd exactly
		{"rounds down below the half", 251, "5", 2, 2, 50},          // 2.51/5 = 0.502 usd = 50.2 cents -> 50
		{"rounds up above the half", 253, "5", 2, 2, 51},            // 2.53/5 = 0.506 usd = 50.6 cents -> 51
		{"exact tie rounds away from zero", 15, "2", 0, 0, 8},       // 15/2 = 7.5 -> 8, not banker's rounding to 8 either way, but proves ties don't truncate to 7
		{"exact tie, negative, rounds away from zero", -15, "2", 0, 0, -8},
		{"0-decimal currency scales against 2-decimal usd", 500, "5", 0, 2, 10000}, // 500 whole units @ 5/usd = 100 usd = 10000 cents
		{"negative amount, non-tie", -253, "5", 2, 2, -51},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := convertToUSDSmallestUnit(tc.amount, tc.unitsPerUSD, tc.currencyDecimals, tc.usdDecimals)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// ---- ListExchangeRatesUseCase ----

func TestListExchangeRatesGroupsByCurrencyWithCurrentAndHistory(t *testing.T) {
	rates := newFakeExchangeRateRepo()
	currencies := newFakeCurrencyRepo("usd", "brl")
	setRate := NewSetExchangeRate(rates, currencies)
	listRates := NewListExchangeRates(rates)
	ctx := context.Background()

	if _, err := setRate.Execute(ctx, SetExchangeRateInput{
		UserID: "u1", Currency: "brl", UnitsPerUSD: "5", EffectiveFrom: mustDate(2026, 1, 1),
	}); err != nil {
		t.Fatalf("seed rate 1: %v", err)
	}
	if _, err := setRate.Execute(ctx, SetExchangeRateInput{
		UserID: "u1", Currency: "brl", UnitsPerUSD: "6", EffectiveFrom: mustDate(2026, 2, 1),
	}); err != nil {
		t.Fatalf("seed rate 2: %v", err)
	}

	groups, err := listRates.Execute(ctx, "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	g := groups[0]
	if g.Currency != "brl" {
		t.Errorf("currency = %q, want brl", g.Currency)
	}
	if len(g.History) != 2 {
		t.Errorf("history has %d rows, want 2", len(g.History))
	}
	if g.Current == nil || g.Current.UnitsPerUSD != "6" {
		t.Errorf("current = %+v, want the 6-rate row (most recent effective_from not in the future)", g.Current)
	}
}

// ---- DeleteExchangeRateUseCase ----

func TestDeleteExchangeRateRequiresOwnership(t *testing.T) {
	rates := newFakeExchangeRateRepo()
	currencies := newFakeCurrencyRepo("usd", "brl")
	setRate := NewSetExchangeRate(rates, currencies)
	deleteRate := NewDeleteExchangeRate(rates)
	ctx := context.Background()

	created, err := setRate.Execute(ctx, SetExchangeRateInput{
		UserID: "userA", Currency: "brl", UnitsPerUSD: "5", EffectiveFrom: mustDate(2026, 1, 1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := deleteRate.Execute(ctx, "userB", created.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("userB delete got %v, want ErrNotFound", err)
	}
	if err := deleteRate.Execute(ctx, "userA", created.ID); err != nil {
		t.Errorf("userA delete: unexpected error: %v", err)
	}
}
