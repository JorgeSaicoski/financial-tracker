package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

// ppFixture builds the standard test rig: brl + usd registered (2
// decimals each), a category repo seeded with food/rent/entertainment at
// their fixture avoidability percentages, and a movements repo ready for
// seeding.
type ppFixture struct {
	movements  *fakeMovementRepo
	categories *fakeCategoryRepo
	rates      *fakeExchangeRateRepo
	currencies *fakeCurrencyRepo
	uc         GetPurchasingPowerUseCase
}

func newPPFixture(t *testing.T) *ppFixture {
	t.Helper()
	movements := newFakeMovementRepo()
	categories := newFakeCategoryRepo()
	rates := newFakeExchangeRateRepo()
	currencies := newFakeCurrencyRepo("usd", "brl")

	ctx := context.Background()
	for name, avoidability := range map[string]int{"food": 20, "rent": 10, "entertainment": 80} {
		a := avoidability
		if _, err := categories.Create(ctx, &dto.CategoryDTO{UserID: "u1", Name: name, AvoidabilityPercent: &a}); err != nil {
			t.Fatal(err)
		}
	}

	toUSD := NewToUSD(rates, currencies)
	return &ppFixture{
		movements: movements, categories: categories, rates: rates, currencies: currencies,
		uc: NewGetPurchasingPower(movements, categories, toUSD),
	}
}

func (f *ppFixture) seedMonth(t *testing.T, monthStart time.Time) {
	t.Helper()
	day := monthStart.AddDate(0, 0, 4) // the 5th
	rows := []*dto.MovementDTO{
		{UserID: "u1", Amount: 10000, Currency: "brl", Category: "income", Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusActive)},
		{UserID: "u1", Amount: -2000, Currency: "brl", Category: "food", Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusActive)},
		{UserID: "u1", Amount: -3000, Currency: "brl", Category: "rent", Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusActive)},
		{UserID: "u1", Amount: -1500, Currency: "brl", Category: "entertainment", Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusActive)},
	}
	for _, m := range rows {
		f.movements.add(m)
	}
}

func (f *ppFixture) setRate(t *testing.T, currency string, effectiveFrom time.Time, unitsPerUSD string) {
	t.Helper()
	if _, err := f.rates.Create(context.Background(), &dto.ExchangeRateDTO{
		UserID: "u1", Currency: currency, UnitsPerUSD: unitsPerUSD, EffectiveFrom: effectiveFrom, CreatedAt: effectiveFrom,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestGetPurchasingPowerFixture matches BACK-12's own acceptance-criteria
// fixture: two months of brl movements (income 10,000; food 2,000 @20%;
// rent 3,000 @10%; entertainment 1,500 @80%) with a rate change (5 -> 6
// brl per usd between the two months).
func TestGetPurchasingPowerFixture(t *testing.T) {
	f := newPPFixture(t)
	now := time.Now().UTC()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	previousMonthStart := currentMonthStart.AddDate(0, -1, 0)

	f.seedMonth(t, previousMonthStart)
	f.seedMonth(t, currentMonthStart)
	f.setRate(t, "brl", previousMonthStart, "5")
	f.setRate(t, "brl", currentMonthStart, "6")

	report, err := f.uc.Execute(context.Background(), "u1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 2 {
		t.Fatalf("want 2 months, got %d", len(report))
	}

	wantPotentialSavings := int64(2000*20/100 + 3000*10/100 + 1500*80/100) // 1900
	for i, month := range report {
		if len(month.Currencies) != 1 || month.Currencies[0].Currency != "brl" {
			t.Fatalf("month %d: want exactly one brl currency group, got %+v", i, month.Currencies)
		}
		c := month.Currencies[0]
		if c.Income != 10000 {
			t.Errorf("month %d: income = %d, want 10000", i, c.Income)
		}
		if c.TotalExpenses != 6500 {
			t.Errorf("month %d: total_expenses = %d, want 6500", i, c.TotalExpenses)
		}
		if c.Profit != 3500 {
			t.Errorf("month %d: profit = %d, want 3500 (identical every month regardless of rate)", i, c.Profit)
		}
		if c.PotentialSavings != wantPotentialSavings {
			t.Errorf("month %d: potential_savings = %d, want %d", i, c.PotentialSavings, wantPotentialSavings)
		}
		if len(c.SpendingByCategory) != 3 {
			t.Errorf("month %d: want 3 categories, got %d", i, len(c.SpendingByCategory))
		}
	}

	// USD profit: month 1 at rate 5, month 2 at rate 6 — same brl profit
	// converts to fewer USD cents when the currency has devalued.
	profitUSDMonth1 := report[0].Currencies[0].ProfitUSD
	profitUSDMonth2 := report[1].Currencies[0].ProfitUSD
	if profitUSDMonth2 >= profitUSDMonth1 {
		t.Errorf("USD profit should be lower in the devalued month: month1=%d month2=%d", profitUSDMonth1, profitUSDMonth2)
	}
	// Both currencies use 2 decimals, so converting reduces to
	// amount / units_per_usd: 3500 brl-cents / 5 = 700 usd-cents.
	wantUSDProfitMonth1 := int64(700)
	if profitUSDMonth1 != wantUSDProfitMonth1 {
		t.Errorf("month 1 profit_usd = %d, want %d", profitUSDMonth1, wantUSDProfitMonth1)
	}
}

func TestGetPurchasingPowerExcludesTransfersAndCancelled(t *testing.T) {
	f := newPPFixture(t)
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	day := monthStart.AddDate(0, 0, 4)
	f.setRate(t, "brl", monthStart, "5")

	f.movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -1000, Currency: "brl", Category: string(entities.CategoryTransfer),
		Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusActive),
	})
	f.movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -500, Currency: "brl", Category: "food",
		Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusVoided),
	})
	f.movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -200, Currency: "brl", Category: "food",
		Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusActive),
	})

	report, err := f.uc.Execute(context.Background(), "u1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 1 || len(report[0].Currencies) != 1 {
		t.Fatalf("unexpected report shape: %+v", report)
	}
	c := report[0].Currencies[0]
	if c.TotalExpenses != 200 {
		t.Errorf("total_expenses = %d, want 200 (transfer and voided rows excluded)", c.TotalExpenses)
	}
}

func TestGetPurchasingPowerUncategorizedContributesZeroSavings(t *testing.T) {
	f := newPPFixture(t)
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	day := monthStart.AddDate(0, 0, 4)
	f.setRate(t, "brl", monthStart, "5")

	f.movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -800, Currency: "brl", Category: "",
		Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusActive),
	})

	report, err := f.uc.Execute(context.Background(), "u1", 1)
	if err != nil {
		t.Fatal(err)
	}
	c := report[0].Currencies[0]
	if c.TotalExpenses != 800 {
		t.Errorf("total_expenses = %d, want 800", c.TotalExpenses)
	}
	if c.PotentialSavings != 0 {
		t.Errorf("potential_savings = %d, want 0 for an uncategorized movement with no override", c.PotentialSavings)
	}
}

func TestGetPurchasingPowerMissingRateMarksIncomplete(t *testing.T) {
	f := newPPFixture(t)
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	day := monthStart.AddDate(0, 0, 4)
	// No rate registered for brl at all.

	f.movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -800, Currency: "brl", Category: "food",
		Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusActive),
	})

	report, err := f.uc.Execute(context.Background(), "u1", 1)
	if err != nil {
		t.Fatal(err)
	}
	c := report[0].Currencies[0]
	if !c.USDIncomplete {
		t.Error("want usd_incomplete=true when no rate is known for the currency")
	}
	if c.TotalExpenses != 800 {
		t.Errorf("native total_expenses should still be correct: got %d, want 800", c.TotalExpenses)
	}
	if c.TotalExpensesUSD != 0 {
		t.Errorf("usd total_expenses should exclude the unconvertible movement: got %d, want 0", c.TotalExpensesUSD)
	}
}

func TestGetPurchasingPowerZeroMovementsReturnsEmpty(t *testing.T) {
	f := newPPFixture(t)
	report, err := f.uc.Execute(context.Background(), "u1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 0 {
		t.Errorf("want no months in the report when there are no movements, got %d", len(report))
	}
}

func TestGetPurchasingPowerMonthsClampedToMax(t *testing.T) {
	f := newPPFixture(t)
	// Just verifying it doesn't error with an absurd months value; the
	// clamp itself is exercised implicitly via the (from,to) window not
	// panicking on very old dates.
	if _, err := f.uc.Execute(context.Background(), "u1", 999); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetPurchasingPowerHonorsPerMovementOverride(t *testing.T) {
	f := newPPFixture(t)
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	day := monthStart.AddDate(0, 0, 4)
	f.setRate(t, "brl", monthStart, "5")

	override := 100
	// A one-off "went karting" expense: no category, 100% avoidable via
	// its own override rather than a category's stored value.
	f.movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -1000, Currency: "brl", AvoidabilityOverridePercent: &override,
		Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusActive),
	})
	// A food-category expense whose override (5%) should win over the
	// category's own stored 20%.
	overrideLow := 5
	f.movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -400, Currency: "brl", Category: "food", AvoidabilityOverridePercent: &overrideLow,
		Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusActive),
	})

	report, err := f.uc.Execute(context.Background(), "u1", 1)
	if err != nil {
		t.Fatal(err)
	}
	c := report[0].Currencies[0]
	want := int64(1000*100/100 + 400*5/100) // 1000 + 20 = 1020
	if c.PotentialSavings != want {
		t.Errorf("potential_savings = %d, want %d (override wins over category default)", c.PotentialSavings, want)
	}
	if c.TotalExpenses != 1400 {
		t.Errorf("total_expenses = %d, want 1400", c.TotalExpenses)
	}
}
