package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

// asFixture mirrors ppFixture (get_purchasing_power_test.go) but for
// GetAvoidabilityScoreUseCase, which needs no exchange rates/ToUSD.
type asFixture struct {
	movements  *fakeMovementRepo
	categories *fakeCategoryRepo
	uc         GetAvoidabilityScoreUseCase
	categoryID map[string]string // name -> id, for seed/tests to reference
}

func newASFixture(t *testing.T) *asFixture {
	t.Helper()
	movements := newFakeMovementRepo()
	categories := newFakeCategoryRepo()

	ctx := context.Background()
	categoryID := make(map[string]string)
	for name, avoidability := range map[string]int{"dining": 80, "market": 20} {
		a := avoidability
		c, err := categories.Create(ctx, &dto.CategoryDTO{Name: name, AvoidabilityPercent: &a, ContributorIDs: []string{"u1"}})
		if err != nil {
			t.Fatal(err)
		}
		categoryID[name] = c.ID
	}

	return &asFixture{
		movements: movements, categories: categories,
		uc:         NewGetAvoidabilityScore(movements, categories),
		categoryID: categoryID,
	}
}

// seed adds one expense movement for user u1, currency brl, category
// name, amount (positive magnitude) on the 5th of monthStart.
func (f *asFixture) seed(monthStart time.Time, category string, amount int64) {
	day := monthStart.AddDate(0, 0, 4)
	id := f.categoryID[category]
	f.movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -amount, Currency: "brl", CategoryID: &id,
		Timestamp: day, CreatedAt: day, Status: string(entities.MovementStatusActive),
	})
}

func monthsAgo(from time.Time, n int) time.Time {
	return time.Date(from.Year(), from.Month()-time.Month(n), 1, 0, 0, 0, 0, time.UTC)
}

// TestAvoidabilityScoreFixture matches the ticket's own acceptance
// fixture: dining at 1,000/1,000/1,000 brl for the 3 preceding months,
// 80% avoidability, 400 brl in the scored month -> reduction_percent=60,
// weighted_score=48.
func TestAvoidabilityScoreFixture(t *testing.T) {
	f := newASFixture(t)
	now := time.Now().UTC()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	for i := 1; i <= 3; i++ {
		f.seed(monthsAgo(currentMonthStart, i), "dining", 1000)
	}
	f.seed(currentMonthStart, "dining", 400)

	report, err := f.uc.Execute(context.Background(), "u1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 1 {
		t.Fatalf("want 1 scored month, got %d", len(report))
	}
	month := report[0]
	if len(month.Currencies) != 1 || month.Currencies[0].Currency != "brl" {
		t.Fatalf("want exactly one brl currency group, got %+v", month.Currencies)
	}
	c := month.Currencies[0]
	if len(c.Categories) != 1 {
		t.Fatalf("want 1 category, got %d: %+v", len(c.Categories), c.Categories)
	}
	dining := c.Categories[0]
	if dining.InsufficientHistory {
		t.Fatal("dining has 3 full prior months of data, should not be insufficient")
	}
	if dining.Baseline != 1000 {
		t.Errorf("baseline = %d, want 1000", dining.Baseline)
	}
	if dining.Actual != 400 {
		t.Errorf("actual = %d, want 400", dining.Actual)
	}
	if dining.ReductionPercent != 60 {
		t.Errorf("reduction_percent = %v, want 60", dining.ReductionPercent)
	}
	if dining.WeightedScore != 48 {
		t.Errorf("weighted_score = %v, want 48", dining.WeightedScore)
	}
	if c.OverallScore != 48 {
		t.Errorf("overall_score = %v, want 48", c.OverallScore)
	}
}

// TestAvoidabilityScoreInsufficientHistoryExcluded covers: a category with
// only one prior month of data is excluded from the score (flagged
// insufficient_history) but still reports its actual spend.
func TestAvoidabilityScoreInsufficientHistoryExcluded(t *testing.T) {
	f := newASFixture(t)
	now := time.Now().UTC()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	f.seed(monthsAgo(currentMonthStart, 1), "market", 500) // only 1 of 3 months
	f.seed(currentMonthStart, "market", 300)

	report, err := f.uc.Execute(context.Background(), "u1", 1)
	if err != nil {
		t.Fatal(err)
	}
	c := report[0].Currencies[0]
	if len(c.Categories) != 1 {
		t.Fatalf("want 1 category, got %d", len(c.Categories))
	}
	market := c.Categories[0]
	if !market.InsufficientHistory {
		t.Error("market has only 1 of 3 prior months, should be insufficient_history")
	}
	if market.Actual != 300 {
		t.Errorf("actual = %d, want 300 (still reported despite insufficient history)", market.Actual)
	}
	if market.Baseline != 0 || market.ReductionPercent != 0 || market.WeightedScore != 0 {
		t.Errorf("insufficient-history category should carry zero baseline/reduction/score, got %+v", market)
	}
	if c.OverallScore != 0 {
		t.Errorf("overall_score = %v, want 0 (insufficient-history category excluded, not zero-contributed)", c.OverallScore)
	}
}

// TestAvoidabilityScoreOverallSumsAcrossCategoriesSameCurrency covers:
// overall score sums correctly across multiple categories in one
// currency, and a category with 2-of-3 data months still counts.
func TestAvoidabilityScoreOverallSumsAcrossCategoriesSameCurrency(t *testing.T) {
	f := newASFixture(t)
	now := time.Now().UTC()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	// dining: full 3-month baseline of 1000, cut to 400 this month (80%
	// avoidable -> weighted_score 48, as in the fixture test above).
	for i := 1; i <= 3; i++ {
		f.seed(monthsAgo(currentMonthStart, i), "dining", 1000)
	}
	f.seed(currentMonthStart, "dining", 400)

	// market: 2 of 3 months have data (500 each), one month skipped
	// entirely; barely moved this month (480), 20% avoidable.
	f.seed(monthsAgo(currentMonthStart, 1), "market", 500)
	f.seed(monthsAgo(currentMonthStart, 3), "market", 500)
	f.seed(currentMonthStart, "market", 480)

	report, err := f.uc.Execute(context.Background(), "u1", 1)
	if err != nil {
		t.Fatal(err)
	}
	c := report[0].Currencies[0]
	if len(c.Categories) != 2 {
		t.Fatalf("want 2 categories, got %d: %+v", len(c.Categories), c.Categories)
	}
	var dining, market CategoryAvoidabilityScore
	for _, cat := range c.Categories {
		switch cat.Category {
		case "dining":
			dining = cat
		case "market":
			market = cat
		}
	}
	if dining.InsufficientHistory {
		t.Fatal("dining should have a full baseline")
	}
	if market.InsufficientHistory {
		t.Fatal("market has 2 of 3 months, should still get a baseline")
	}
	// market baseline = (500+0+500)/3 = 333 (integer division; the
	// missing month contributes 0 to the fixed 3-month window).
	if market.Baseline != 333 {
		t.Errorf("market baseline = %d, want 333", market.Baseline)
	}

	wantOverall := dining.WeightedScore + market.WeightedScore
	if c.OverallScore != wantOverall {
		t.Errorf("overall_score = %v, want %v (sum of both categories' weighted scores)", c.OverallScore, wantOverall)
	}
}

// TestAvoidabilityScoreSecondCurrencyReportedSeparately covers: a second
// currency in the same month is its own entry, never blended with the
// first.
func TestAvoidabilityScoreSecondCurrencyReportedSeparately(t *testing.T) {
	f := newASFixture(t)
	now := time.Now().UTC()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	for i := 1; i <= 3; i++ {
		f.seed(monthsAgo(currentMonthStart, i), "dining", 1000)
	}
	f.seed(currentMonthStart, "dining", 400)

	// A usd movement the same month — must not blend into the brl figures.
	dining := f.categoryID["dining"]
	f.movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -200, Currency: "usd", CategoryID: &dining,
		Timestamp: currentMonthStart.AddDate(0, 0, 4), CreatedAt: currentMonthStart, Status: string(entities.MovementStatusActive),
	})

	report, err := f.uc.Execute(context.Background(), "u1", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(report[0].Currencies) != 2 {
		t.Fatalf("want 2 currency groups, got %d", len(report[0].Currencies))
	}
	for _, c := range report[0].Currencies {
		if c.Currency == "usd" {
			if len(c.Categories) != 1 || !c.Categories[0].InsufficientHistory {
				t.Errorf("usd dining should be insufficient_history (no prior usd months), got %+v", c.Categories)
			}
		}
	}
}

// TestAvoidabilityScoreNonOverlappingBaselines covers: two consecutive
// scored months for the same category use non-overlapping baselines —
// month M's baseline never includes month M itself.
func TestAvoidabilityScoreNonOverlappingBaselines(t *testing.T) {
	f := newASFixture(t)
	now := time.Now().UTC()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	previousMonthStart := monthsAgo(currentMonthStart, 1)

	// 4 months of history so both the previous and current scored month
	// each have a full 3-month baseline behind them.
	for i := 2; i <= 4; i++ {
		f.seed(monthsAgo(currentMonthStart, i), "dining", 1000)
	}
	f.seed(previousMonthStart, "dining", 800)
	f.seed(currentMonthStart, "dining", 400)

	report, err := f.uc.Execute(context.Background(), "u1", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 2 {
		t.Fatalf("want 2 scored months, got %d", len(report))
	}
	prevScore := report[0].Currencies[0].Categories[0]
	currScore := report[1].Currencies[0].Categories[0]

	// Previous month's baseline is the 3 months before it (all 1000s),
	// unaffected by its own 800 actual.
	if prevScore.Baseline != 1000 {
		t.Errorf("previous month baseline = %d, want 1000 (must not include its own actual)", prevScore.Baseline)
	}
	// Current month's baseline includes the previous month's actual
	// (800) plus 2 of the older 1000s — not the same window as above.
	if currScore.Baseline == prevScore.Baseline {
		t.Errorf("current month baseline (%d) should differ from previous month's (%d) — non-overlapping windows", currScore.Baseline, prevScore.Baseline)
	}
}

func TestAvoidabilityScoreZeroMovementsReturnsEmptyCurrencies(t *testing.T) {
	f := newASFixture(t)
	report, err := f.uc.Execute(context.Background(), "u1", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 3 {
		t.Fatalf("want 3 months, got %d", len(report))
	}
	for _, m := range report {
		if len(m.Currencies) != 0 {
			t.Errorf("expected no currency groups with zero movements, got %+v", m.Currencies)
		}
	}
}

func TestAvoidabilityScoreMonthsClampedToMax(t *testing.T) {
	f := newASFixture(t)
	report, err := f.uc.Execute(context.Background(), "u1", 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != maxAvoidabilityScoreMonths {
		t.Errorf("want %d months (clamped), got %d", maxAvoidabilityScoreMonths, len(report))
	}
}
