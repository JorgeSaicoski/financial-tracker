package usecases

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/pkg/errors"
)

type setExchangeRateUseCase struct {
	rates      repositories.ExchangeRateRepository
	currencies repositories.CurrencyRepository
}

// NewSetExchangeRate returns interface type for dependency injection.
func NewSetExchangeRate(rates repositories.ExchangeRateRepository, currencies repositories.CurrencyRepository) SetExchangeRateUseCase {
	return &setExchangeRateUseCase{rates: rates, currencies: currencies}
}

func (uc *setExchangeRateUseCase) Execute(ctx context.Context, input SetExchangeRateInput) (*dto.ExchangeRateDTO, error) {
	currency := strings.ToLower(strings.TrimSpace(input.Currency))
	if input.UserID == "" || currency == "" {
		return nil, fmt.Errorf("%w: currency is required", apperrors.ErrInvalidInput)
	}
	if currency == "usd" {
		return nil, fmt.Errorf("%w: usd is the reference currency and needs no rate", apperrors.ErrInvalidInput)
	}

	registered, err := uc.currencies.List(ctx)
	if err != nil {
		return nil, err
	}
	if !contains(registered, currency) {
		return nil, fmt.Errorf("%w: unknown currency %q (add it via POST /currencies first)", apperrors.ErrInvalidInput, input.Currency)
	}

	unitsPerUSD := strings.TrimSpace(input.UnitsPerUSD)
	rate, ok := new(big.Rat).SetString(unitsPerUSD)
	if !ok || rate.Sign() <= 0 {
		return nil, fmt.Errorf("%w: units_per_usd must be a positive decimal number", apperrors.ErrInvalidInput)
	}

	// Rates are dated, not timestamped: normalize to midnight UTC so a
	// second POST for "the same day" reliably hits the upsert's unique
	// key instead of drifting on time-of-day.
	effectiveFrom := input.EffectiveFrom
	if effectiveFrom.IsZero() {
		effectiveFrom = time.Now().UTC()
	}
	effectiveFrom = time.Date(effectiveFrom.Year(), effectiveFrom.Month(), effectiveFrom.Day(), 0, 0, 0, 0, time.UTC)

	return uc.rates.Create(ctx, &dto.ExchangeRateDTO{
		UserID:        input.UserID,
		Currency:      currency,
		UnitsPerUSD:   unitsPerUSD,
		EffectiveFrom: effectiveFrom,
		CreatedAt:     time.Now().UTC(),
	})
}

type listExchangeRatesUseCase struct {
	rates repositories.ExchangeRateRepository
}

// NewListExchangeRates returns interface type for dependency injection.
func NewListExchangeRates(rates repositories.ExchangeRateRepository) ListExchangeRatesUseCase {
	return &listExchangeRatesUseCase{rates: rates}
}

func (uc *listExchangeRatesUseCase) Execute(ctx context.Context, userID string) ([]ExchangeRateGroup, error) {
	rows, err := uc.rates.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	// rows arrive ordered "currency ASC, effective_from DESC" (repository
	// contract), so a single pass both groups by currency (in that
	// alphabetical order) and finds each currency's Current: the first
	// row, walking newest-first, whose EffectiveFrom isn't in the future.
	now := time.Now().UTC()
	order := make([]string, 0)
	groups := make(map[string]*ExchangeRateGroup)
	for _, r := range rows {
		g, ok := groups[r.Currency]
		if !ok {
			g = &ExchangeRateGroup{Currency: r.Currency}
			groups[r.Currency] = g
			order = append(order, r.Currency)
		}
		g.History = append(g.History, r)
		if g.Current == nil && !r.EffectiveFrom.After(now) {
			g.Current = r
		}
	}

	out := make([]ExchangeRateGroup, 0, len(order))
	for _, currency := range order {
		out = append(out, *groups[currency])
	}
	return out, nil
}

type deleteExchangeRateUseCase struct {
	rates repositories.ExchangeRateRepository
}

// NewDeleteExchangeRate returns interface type for dependency injection.
func NewDeleteExchangeRate(rates repositories.ExchangeRateRepository) DeleteExchangeRateUseCase {
	return &deleteExchangeRateUseCase{rates: rates}
}

func (uc *deleteExchangeRateUseCase) Execute(ctx context.Context, userID, id string) error {
	if userID == "" || id == "" {
		return apperrors.ErrInvalidInput
	}
	return uc.rates.Delete(ctx, userID, id)
}

type toUSDUseCase struct {
	rates      repositories.ExchangeRateRepository
	currencies repositories.CurrencyRepository
}

// NewToUSD returns interface type for dependency injection.
func NewToUSD(rates repositories.ExchangeRateRepository, currencies repositories.CurrencyRepository) ToUSDUseCase {
	return &toUSDUseCase{rates: rates, currencies: currencies}
}

func (uc *toUSDUseCase) Execute(ctx context.Context, userID string, amount int64, currency string, at time.Time) (int64, error) {
	currency = strings.ToLower(strings.TrimSpace(currency))
	if currency == "usd" {
		return amount, nil
	}

	rate, err := uc.rates.RateAt(ctx, userID, currency, at)
	if err != nil {
		if apperrors.Is(err, apperrors.ErrNotFound) {
			return 0, fmt.Errorf("%w: no exchange rate known for %s at or before %s",
				apperrors.ErrNotFound, currency, at.Format(time.RFC3339))
		}
		return 0, err
	}

	currencyDecimals, err := uc.currencies.Decimals(ctx, currency)
	if err != nil {
		return 0, err
	}
	usdDecimals, err := uc.currencies.Decimals(ctx, "usd")
	if err != nil {
		return 0, err
	}

	return convertToUSDSmallestUnit(amount, rate.UnitsPerUSD, currencyDecimals, usdDecimals)
}

// convertToUSDSmallestUnit converts amount (currency's smallest unit) to
// USD's smallest unit, given "1 usd = unitsPerUSD major units of
// currency" and each side's decimal places. All math is done over exact
// rationals (math/big), never float64, and the final division rounds half
// away from zero — so a value converted through this function is
// reproducible to the cent, not an approximation.
func convertToUSDSmallestUnit(amount int64, unitsPerUSD string, currencyDecimals, usdDecimals int) (int64, error) {
	rate, ok := new(big.Rat).SetString(strings.TrimSpace(unitsPerUSD))
	if !ok || rate.Sign() <= 0 {
		return 0, fmt.Errorf("invalid stored exchange rate %q", unitsPerUSD)
	}

	// result = amount * 10^usdDecimals / (10^currencyDecimals * rate)
	//        = amount * 10^usdDecimals * rate.Denom() / (10^currencyDecimals * rate.Num())
	numerator := new(big.Int).Mul(big.NewInt(amount), pow10(usdDecimals))
	numerator.Mul(numerator, rate.Denom())
	denominator := new(big.Int).Mul(pow10(currencyDecimals), rate.Num())

	return roundHalfAwayFromZero(numerator, denominator), nil
}

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

// roundHalfAwayFromZero divides num/den (den > 0) and rounds ties away
// from zero, e.g. 2.5 -> 3, -2.5 -> -3.
func roundHalfAwayFromZero(num, den *big.Int) int64 {
	neg := num.Sign() < 0
	n := new(big.Int).Abs(num)

	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(n, den, rem)

	twiceRem := new(big.Int).Lsh(rem, 1) // rem * 2
	if twiceRem.Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if neg {
		q.Neg(q)
	}
	return q.Int64()
}
