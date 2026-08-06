package usecases

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type processBillingWebhookUseCase struct {
	subscriptions repositories.SubscriptionRepository
	settings      repositories.UserSettingsRepository
}

// NewProcessBillingWebhook returns interface type for dependency injection.
func NewProcessBillingWebhook(subscriptions repositories.SubscriptionRepository, settings repositories.UserSettingsRepository) ProcessBillingWebhookUseCase {
	return &processBillingWebhookUseCase{subscriptions: subscriptions, settings: settings}
}

func (uc *processBillingWebhookUseCase) Execute(ctx context.Context, input ProcessBillingWebhookInput) (*dto.SubscriptionDTO, error) {
	if input.UserID == "" || input.Provider == "" || input.ProviderSubscriptionID == "" {
		return nil, fmt.Errorf("%w: user_id, provider, and provider_subscription_id are required", apperrors.ErrInvalidInput)
	}
	if !dto.ValidSubscriptionStatus(input.Status) {
		return nil, fmt.Errorf("%w: unknown subscription status %q", apperrors.ErrInvalidInput, input.Status)
	}
	if input.CurrentPeriodEnd.IsZero() {
		return nil, fmt.Errorf("%w: current_period_end is required", apperrors.ErrInvalidInput)
	}

	sub, err := uc.subscriptions.Upsert(ctx, &dto.SubscriptionDTO{
		UserID:                 input.UserID,
		Provider:               input.Provider,
		ProviderSubscriptionID: input.ProviderSubscriptionID,
		Status:                 input.Status,
		CurrentPeriodEnd:       input.CurrentPeriodEnd,
	})
	if err != nil {
		return nil, err
	}

	// Only active takes effect immediately — gaining access on successful
	// payment should never wait. Losing it is the opposite: neither
	// canceled nor past_due touches entitlement here at all (BACK-19's
	// acceptance criteria are explicit that even an explicit cancellation
	// "flips it back after the grace period, not immediately" — treating
	// cancellation as urgent while a late card gets leniency would be an
	// odd asymmetry). The grace-period sweep
	// (internal/application/billing) is what eventually flips either
	// status to false once CurrentPeriodEnd + grace period elapses.
	if input.Status == dto.SubscriptionStatusActive {
		if _, err := uc.settings.SetCloudStorageEntitled(ctx, input.UserID, true); err != nil {
			return nil, err
		}
	}

	return sub, nil
}

type getBillingPlanUseCase struct {
	rates             repositories.ExchangeRateRepository
	currencies        repositories.CurrencyRepository
	referencePriceUSD int64
}

// NewGetBillingPlan returns interface type for dependency injection.
// referencePriceUSDCents is the annual price in USD's smallest unit
// (e.g. 1000 = $10.00/year) — a build-time/env-configured constant, not
// hardcoded here (see cmd/api's BILLING_REFERENCE_PRICE_USD_CENTS).
func NewGetBillingPlan(rates repositories.ExchangeRateRepository, currencies repositories.CurrencyRepository, referencePriceUSDCents int64) GetBillingPlanUseCase {
	return &getBillingPlanUseCase{rates: rates, currencies: currencies, referencePriceUSD: referencePriceUSDCents}
}

func (uc *getBillingPlanUseCase) Execute(ctx context.Context, userID, currency string) (BillingPlanView, error) {
	currency = strings.ToLower(strings.TrimSpace(currency))
	if currency == "" || currency == "usd" {
		return BillingPlanView{Currency: "usd", Amount: uc.referencePriceUSD}, nil
	}

	rate, err := uc.rates.RateAt(ctx, userID, currency, time.Now().UTC())
	if err != nil {
		if apperrors.Is(err, apperrors.ErrNotFound) {
			// Documented fallback (BACK-19): no rate known for this
			// currency, so the reference currency is returned instead of
			// a wrong or hardcoded conversion — never assume USD is
			// universal, but also never invent a number.
			return BillingPlanView{Currency: "usd", Amount: uc.referencePriceUSD}, nil
		}
		return BillingPlanView{}, err
	}

	usdDecimals, err := uc.currencies.Decimals(ctx, "usd")
	if err != nil {
		return BillingPlanView{}, err
	}
	currencyDecimals, err := uc.currencies.Decimals(ctx, currency)
	if err != nil {
		return BillingPlanView{}, err
	}

	amount, err := convertFromUSDSmallestUnit(uc.referencePriceUSD, rate.UnitsPerUSD, usdDecimals, currencyDecimals)
	if err != nil {
		return BillingPlanView{}, err
	}
	return BillingPlanView{Currency: currency, Amount: amount}, nil
}

// convertFromUSDSmallestUnit converts amountUSD (USD's smallest unit) to
// currency's smallest unit, given "1 usd = unitsPerUSD major units of
// currency" — the inverse of convertToUSDSmallestUnit (exchange_rates.go),
// sharing its exact-rational math (math/big, never float64) and
// half-away-from-zero rounding.
func convertFromUSDSmallestUnit(amountUSD int64, unitsPerUSD string, usdDecimals, currencyDecimals int) (int64, error) {
	rate, ok := new(big.Rat).SetString(strings.TrimSpace(unitsPerUSD))
	if !ok || rate.Sign() <= 0 {
		return 0, fmt.Errorf("invalid stored exchange rate %q", unitsPerUSD)
	}

	// result = amountUSD * 10^currencyDecimals * rate
	//        = amountUSD * 10^currencyDecimals * rate.Num() / (10^usdDecimals * rate.Denom())
	numerator := new(big.Int).Mul(big.NewInt(amountUSD), pow10(currencyDecimals))
	numerator.Mul(numerator, rate.Num())
	denominator := new(big.Int).Mul(pow10(usdDecimals), rate.Denom())

	result := roundHalfAwayFromZero(numerator, denominator)
	if !result.IsInt64() {
		return 0, fmt.Errorf("converted amount %s overflows int64", result.String())
	}
	return result.Int64(), nil
}
