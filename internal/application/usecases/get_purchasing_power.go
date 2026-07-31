package usecases

import (
	"context"
	"sort"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

const (
	defaultPurchasingPowerMonths = 6
	maxPurchasingPowerMonths     = 24
)

type getPurchasingPowerUseCase struct {
	movements  repositories.MovementRepository
	categories repositories.CategoryRepository
	toUSD      ToUSDUseCase
}

// NewGetPurchasingPower returns interface type for dependency injection.
func NewGetPurchasingPower(movements repositories.MovementRepository, categories repositories.CategoryRepository, toUSD ToUSDUseCase) GetPurchasingPowerUseCase {
	return &getPurchasingPowerUseCase{movements: movements, categories: categories, toUSD: toUSD}
}

// monthKey is a (year, month) bucket key, and currencyBucket groups one
// month's movements by native currency (BACK-12 never sums natively
// across currencies).
type monthKey struct {
	year  int
	month time.Month
}

func (uc *getPurchasingPowerUseCase) Execute(ctx context.Context, userID string, months int) ([]PurchasingPowerMonth, error) {
	if userID == "" {
		return nil, apperrors.ErrInvalidInput
	}
	if months <= 0 {
		months = defaultPurchasingPowerMonths
	}
	if months > maxPurchasingPowerMonths {
		months = maxPurchasingPowerMonths
	}

	now := time.Now().UTC()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	from := time.Date(currentMonthStart.Year(), currentMonthStart.Month()-time.Month(months-1), 1, 0, 0, 0, 0, time.UTC)
	to := currentMonthStart.AddDate(0, 1, 0) // exclusive: start of next month

	movements, err := uc.movements.ListByUser(ctx, userID, nil, &from, &to, 0, 0)
	if err != nil {
		return nil, err
	}

	categoryRows, err := uc.categories.ListForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	categoriesByID := CategoriesByID(categoryRows)

	byMonth, monthOrder := bucketByMonthAndCurrency(movements)

	out := make([]PurchasingPowerMonth, 0, len(monthOrder))
	for _, key := range monthOrder {
		monthReport, err := uc.buildMonth(ctx, userID, key, byMonth[key], categoriesByID, now)
		if err != nil {
			return nil, err
		}
		out = append(out, monthReport)
	}
	return out, nil
}

func (uc *getPurchasingPowerUseCase) buildMonth(
	ctx context.Context, userID string, key monthKey, byCurrency map[string][]*dto.MovementDTO,
	categoriesByID map[string]*dto.CategoryDTO, now time.Time,
) (PurchasingPowerMonth, error) {
	month := PurchasingPowerMonth{Month: time.Date(key.year, key.month, 1, 0, 0, 0, 0, time.UTC)}

	currencies := currenciesActiveIn(byCurrency)

	var profitUSDAtCurrentRate int64
	for _, currency := range currencies {
		view, currentRateSum, err := uc.buildCurrencyView(ctx, userID, currency, byCurrency[currency], categoriesByID, now)
		if err != nil {
			return PurchasingPowerMonth{}, err
		}
		month.Currencies = append(month.Currencies, view)
		profitUSDAtCurrentRate += currentRateSum
	}
	month.ProfitUSDAtCurrentRate = profitUSDAtCurrentRate
	return month, nil
}

// buildCurrencyView computes one month's native + historical-rate-USD
// figures for one currency, plus that group's contribution to
// profit_usd_at_current_rate (summed by the caller across every
// currency active that month).
func (uc *getPurchasingPowerUseCase) buildCurrencyView(
	ctx context.Context, userID, currency string, rows []*dto.MovementDTO,
	categoriesByID map[string]*dto.CategoryDTO, now time.Time,
) (PurchasingPowerCurrencyView, int64, error) {
	view := PurchasingPowerCurrencyView{Currency: currency}
	var currentRateProfit int64

	for _, m := range rows {
		isIncome := m.Amount > 0

		if isIncome {
			view.Income += m.Amount
		} else {
			expense := -m.Amount
			view.TotalExpenses += expense

			view.PotentialSavings += entities.PotentialSaving(expense, EffectiveAvoidability(m, categoriesByID))
		}

		usdAmount, err := uc.toUSD.Execute(ctx, userID, m.Amount, currency, m.Timestamp)
		if err != nil {
			if apperrors.Is(err, apperrors.ErrNotFound) {
				view.USDIncomplete = true
			} else {
				return PurchasingPowerCurrencyView{}, 0, err
			}
		} else {
			if isIncome {
				view.IncomeUSD += usdAmount
			} else {
				view.TotalExpensesUSD += -usdAmount
				view.PotentialSavingsUSD += entities.PotentialSaving(-usdAmount, EffectiveAvoidability(m, categoriesByID))
			}
		}

		// profit_usd_at_current_rate: same amount, converted at "now"
		// instead of the movement's own timestamp. A currency with no
		// rate as of now simply doesn't contribute to this supplementary
		// figure (the primary USDIncomplete signal above already covers
		// the historical-rate figures this field is compared against).
		if currentUSDAmount, err := uc.toUSD.Execute(ctx, userID, m.Amount, currency, now); err == nil {
			currentRateProfit += currentUSDAmount
		} else if !apperrors.Is(err, apperrors.ErrNotFound) {
			return PurchasingPowerCurrencyView{}, 0, err
		}
	}

	spending := categorySpending(rows)
	ids := make([]string, 0, len(spending))
	for id := range spending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		var avoidabilityPercent *int
		var categoryName string
		if c, ok := categoriesByID[id]; ok {
			avoidabilityPercent = c.AvoidabilityPercent
			categoryName = c.Name
		}
		view.SpendingByCategory = append(view.SpendingByCategory, PurchasingPowerCategorySpending{
			Category: categoryName, AvoidabilityPercent: avoidabilityPercent, Amount: spending[id],
		})
	}
	view.Profit = view.Income - view.TotalExpenses
	view.ProfitUSD = view.IncomeUSD - view.TotalExpensesUSD
	return view, currentRateProfit, nil
}

// bucketByMonthAndCurrency groups non-voided, non-transfer movements by
// calendar month then native currency — the same grouping/exclusion rule
// BACK-12's report and BACK-18's avoidability score both need (extracted
// per BACK-18's own instruction to share this rather than recompute it).
// monthOrder preserves first-seen order (oldest to newest, since
// ListByUser returns newest-first and this walks it in that order) so a
// caller's response doesn't depend on Go's random map iteration order.
func bucketByMonthAndCurrency(movements []*dto.MovementDTO) (map[monthKey]map[string][]*dto.MovementDTO, []monthKey) {
	byMonth := make(map[monthKey]map[string][]*dto.MovementDTO)
	var monthOrder []monthKey
	for _, m := range movements {
		if m.Status == string(entities.MovementStatusVoided) {
			continue
		}
		if m.CategoryID != nil && *m.CategoryID == entities.CategoryTransferID {
			// Transfers (including to an investment-type account) are
			// neither income nor expense — excluding them here also
			// excludes contributions to investment accounts, satisfying
			// "profit not considering investments" without a separate
			// account-type check.
			continue
		}
		key := monthKey{year: m.Timestamp.Year(), month: m.Timestamp.Month()}
		byCurrency, ok := byMonth[key]
		if !ok {
			byCurrency = make(map[string][]*dto.MovementDTO)
			byMonth[key] = byCurrency
			monthOrder = append(monthOrder, key)
		}
		byCurrency[m.Currency] = append(byCurrency[m.Currency], m)
	}
	sort.Slice(monthOrder, func(i, j int) bool {
		if monthOrder[i].year != monthOrder[j].year {
			return monthOrder[i].year < monthOrder[j].year
		}
		return monthOrder[i].month < monthOrder[j].month
	})
	return byMonth, monthOrder
}

// currenciesActiveIn returns the sorted set of native currencies present
// in one month's movement bucket — sorted so a caller's response doesn't
// depend on Go's random map iteration order.
func currenciesActiveIn(byCurrency map[string][]*dto.MovementDTO) []string {
	currencies := make([]string, 0, len(byCurrency))
	for currency := range byCurrency {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	return currencies
}

// categorySpending sums each category's expense magnitude (positive,
// smallest currency unit) from a set of movements already bucketed to one
// calendar month + currency via bucketByMonthAndCurrency — shared by
// BACK-12's spending_by_category and BACK-18's baseline/actual figures.
// Keyed by category id ("" for uncategorized); income rows (Amount > 0)
// don't contribute.
func categorySpending(rows []*dto.MovementDTO) map[string]int64 {
	spending := make(map[string]int64)
	for _, m := range rows {
		if m.Amount > 0 {
			continue
		}
		categoryKey := ""
		if m.CategoryID != nil {
			categoryKey = *m.CategoryID
		}
		spending[categoryKey] += -m.Amount
	}
	return spending
}
