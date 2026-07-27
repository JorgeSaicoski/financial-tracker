package usecases

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

const (
	defaultAvoidabilityScoreMonths = 6
	maxAvoidabilityScoreMonths     = 24
	// avoidabilityBaselineMonths is the fixed trailing window a category's
	// baseline averages over — the ticket's own wording ("the trailing
	// average ... over the 3 preceding full calendar months"), so the
	// denominator below is always 3, not the count of months that
	// happened to have data (see avoidabilityMinBaselineMonths for the
	// separate "is this baseline trustworthy at all" gate).
	avoidabilityBaselineMonths = 3
	// avoidabilityMinBaselineMonths is the minimum number of the 3
	// baseline months that must have any data for a category before its
	// baseline is trusted — never fabricated from a single data point.
	avoidabilityMinBaselineMonths = 2
)

type getAvoidabilityScoreUseCase struct {
	movements  repositories.MovementRepository
	categories repositories.CategoryRepository
}

// NewGetAvoidabilityScore returns interface type for dependency injection.
func NewGetAvoidabilityScore(movements repositories.MovementRepository, categories repositories.CategoryRepository) GetAvoidabilityScoreUseCase {
	return &getAvoidabilityScoreUseCase{movements: movements, categories: categories}
}

func (uc *getAvoidabilityScoreUseCase) Execute(ctx context.Context, userID string, months int) ([]AvoidabilityScoreMonth, error) {
	if userID == "" {
		return nil, apperrors.ErrInvalidInput
	}
	if months <= 0 {
		months = defaultAvoidabilityScoreMonths
	}
	if months > maxAvoidabilityScoreMonths {
		months = maxAvoidabilityScoreMonths
	}

	now := time.Now().UTC()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	earliestScored := time.Date(currentMonthStart.Year(), currentMonthStart.Month()-time.Month(months-1), 1, 0, 0, 0, 0, time.UTC)
	// The earliest scored month needs its own 3 preceding months of
	// history for a baseline, so the query window reaches further back
	// than the scored months themselves.
	from := time.Date(earliestScored.Year(), earliestScored.Month()-avoidabilityBaselineMonths, 1, 0, 0, 0, 0, time.UTC)
	to := currentMonthStart.AddDate(0, 1, 0) // exclusive: start of next month

	movements, err := uc.movements.ListByUser(ctx, userID, nil, &from, &to, 0, 0)
	if err != nil {
		return nil, err
	}
	categoryRows, err := uc.categories.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	categoriesByName := CategoriesByName(categoryRows)

	byMonth, _ := bucketByMonthAndCurrency(movements)

	out := make([]AvoidabilityScoreMonth, 0, months)
	for i := months - 1; i >= 0; i-- {
		monthDate := time.Date(currentMonthStart.Year(), currentMonthStart.Month()-time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		key := monthKey{year: monthDate.Year(), month: monthDate.Month()}
		out = append(out, uc.buildScoredMonth(monthDate, key, byMonth, categoriesByName))
	}
	return out, nil
}

// buildScoredMonth computes one scored month's per-currency, per-category
// follow-through figures against each category's own 3-preceding-month
// baseline.
func (uc *getAvoidabilityScoreUseCase) buildScoredMonth(
	monthDate time.Time, key monthKey, byMonth map[monthKey]map[string][]*dto.MovementDTO,
	categoriesByName map[string]*dto.CategoryDTO,
) AvoidabilityScoreMonth {
	report := AvoidabilityScoreMonth{Month: monthDate}

	baselineKeys := make([]monthKey, avoidabilityBaselineMonths)
	for i := 1; i <= avoidabilityBaselineMonths; i++ {
		bmDate := time.Date(key.year, key.month-time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		baselineKeys[i-1] = monthKey{year: bmDate.Year(), month: bmDate.Month()}
	}

	// Every currency active this scored month, or in any of its 3
	// baseline months — a currency the user only used historically still
	// needs its categories' baselines computed even if this month has
	// nothing in it (actual = 0 across the board).
	currencySet := map[string]bool{}
	for currency := range byMonth[key] {
		currencySet[currency] = true
	}
	for _, bk := range baselineKeys {
		for currency := range byMonth[bk] {
			currencySet[currency] = true
		}
	}
	currencies := make([]string, 0, len(currencySet))
	for c := range currencySet {
		currencies = append(currencies, c)
	}
	sort.Strings(currencies)

	for _, currency := range currencies {
		actualSpend := categorySpending(byMonth[key][currency])
		baselineSpends := make([]map[string]int64, avoidabilityBaselineMonths)
		for i, bk := range baselineKeys {
			baselineSpends[i] = categorySpending(byMonth[bk][currency])
		}

		categorySet := map[string]bool{}
		for name := range actualSpend {
			categorySet[name] = true
		}
		for _, bs := range baselineSpends {
			for name := range bs {
				categorySet[name] = true
			}
		}
		names := make([]string, 0, len(categorySet))
		for name := range categorySet {
			names = append(names, name)
		}
		sort.Strings(names)

		currencyView := AvoidabilityScoreCurrencyView{Currency: currency}
		var overall float64
		for _, name := range names {
			var avoidabilityPercent *int
			if c, ok := categoriesByName[strings.ToLower(name)]; ok {
				avoidabilityPercent = c.AvoidabilityPercent
			}
			score := CategoryAvoidabilityScore{
				Category:            name,
				AvoidabilityPercent: avoidabilityPercent,
				Actual:              actualSpend[name],
			}

			var sum int64
			var dataMonths int
			for _, bs := range baselineSpends {
				if v, ok := bs[name]; ok {
					sum += v
					dataMonths++
				}
			}
			if dataMonths < avoidabilityMinBaselineMonths {
				score.InsufficientHistory = true
			} else {
				baseline := sum / int64(avoidabilityBaselineMonths)
				score.Baseline = baseline
				// baseline can round down to 0 for a very small trailing
				// spend (integer division); guard the divide rather than
				// let a genuinely rare case panic.
				if baseline != 0 {
					reduction := float64(baseline-score.Actual) / float64(baseline) * 100
					score.ReductionPercent = reduction
					if avoidabilityPercent != nil {
						score.WeightedScore = reduction * float64(*avoidabilityPercent) / 100
						overall += score.WeightedScore
					}
				}
			}
			currencyView.Categories = append(currencyView.Categories, score)
		}
		currencyView.OverallScore = overall
		report.Currencies = append(report.Currencies, currencyView)
	}
	return report
}
