package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestProcessBillingWebhookRejectsUnknownStatus(t *testing.T) {
	uc := NewProcessBillingWebhook(newFakeSubscriptionRepo(), newFakeUserSettingsRepo())
	_, err := uc.Execute(context.Background(), ProcessBillingWebhookInput{
		UserID: "u1", Provider: "stripe", ProviderSubscriptionID: "sub_1",
		Status: "not-a-real-status", CurrentPeriodEnd: time.Now(),
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

// TestProcessBillingWebhookActiveFlipsEntitledTrue is BACK-19's "one
// webhook round-trip" acceptance criterion.
func TestProcessBillingWebhookActiveFlipsEntitledTrue(t *testing.T) {
	subs := newFakeSubscriptionRepo()
	settings := newFakeUserSettingsRepo()
	uc := NewProcessBillingWebhook(subs, settings)
	ctx := context.Background()

	sub, err := uc.Execute(ctx, ProcessBillingWebhookInput{
		UserID: "u1", Provider: "stripe", ProviderSubscriptionID: "sub_1",
		Status: dto.SubscriptionStatusActive, CurrentPeriodEnd: time.Now().AddDate(1, 0, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sub.Status != dto.SubscriptionStatusActive {
		t.Errorf("subscription status = %q, want active", sub.Status)
	}

	s, err := settings.Get(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !s.CloudStorageEntitled {
		t.Error("cloud_storage_entitled should be true after an active webhook")
	}
}

// TestProcessBillingWebhookCanceledDoesNotFlipEntitlementImmediately is
// BACK-19's acceptance criterion in its own words: "cancelling flips it
// back after the grace period, not immediately" — an explicit
// cancellation gets the same leniency as a late payment, not instant
// cutoff. Only the grace-period sweep (internal/application/billing) may
// actually lapse it.
func TestProcessBillingWebhookCanceledDoesNotFlipEntitlementImmediately(t *testing.T) {
	subs := newFakeSubscriptionRepo()
	settings := newFakeUserSettingsRepo()
	uc := NewProcessBillingWebhook(subs, settings)
	ctx := context.Background()

	if _, err := uc.Execute(ctx, ProcessBillingWebhookInput{
		UserID: "u1", Provider: "stripe", ProviderSubscriptionID: "sub_1",
		Status: dto.SubscriptionStatusActive, CurrentPeriodEnd: time.Now().AddDate(1, 0, 0),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Execute(ctx, ProcessBillingWebhookInput{
		UserID: "u1", Provider: "stripe", ProviderSubscriptionID: "sub_1",
		Status: dto.SubscriptionStatusCanceled, CurrentPeriodEnd: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	s, err := settings.Get(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !s.CloudStorageEntitled {
		t.Error("cloud_storage_entitled must stay true immediately after cancellation — the grace-period sweep lapses it later, not this webhook")
	}

	sub, err := subs.GetByUserID(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if sub.Status != dto.SubscriptionStatusCanceled {
		t.Errorf("subscription status should still record canceled = %q, want canceled", sub.Status)
	}
}

// TestProcessBillingWebhookPastDueDoesNotFlipEntitlementYet: "a late card
// shouldn't cut off access instantly" — only the grace-period sweep
// (internal/application/billing) may flip entitlement for past_due.
func TestProcessBillingWebhookPastDueDoesNotFlipEntitlementYet(t *testing.T) {
	subs := newFakeSubscriptionRepo()
	settings := newFakeUserSettingsRepo()
	uc := NewProcessBillingWebhook(subs, settings)
	ctx := context.Background()

	if _, err := uc.Execute(ctx, ProcessBillingWebhookInput{
		UserID: "u1", Provider: "stripe", ProviderSubscriptionID: "sub_1",
		Status: dto.SubscriptionStatusActive, CurrentPeriodEnd: time.Now().AddDate(1, 0, 0),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Execute(ctx, ProcessBillingWebhookInput{
		UserID: "u1", Provider: "stripe", ProviderSubscriptionID: "sub_1",
		Status: dto.SubscriptionStatusPastDue, CurrentPeriodEnd: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	s, err := settings.Get(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if !s.CloudStorageEntitled {
		t.Error("cloud_storage_entitled must stay true immediately after going past_due (grace period)")
	}
}

func TestGetBillingPlanDefaultsToUSDReferencePrice(t *testing.T) {
	uc := NewGetBillingPlan(newFakeExchangeRateRepo(), newFakeCurrencyRepo(), 1000)
	view, err := uc.Execute(context.Background(), "u1", "")
	if err != nil {
		t.Fatal(err)
	}
	if view.Currency != "usd" || view.Amount != 1000 {
		t.Errorf("view = %+v, want usd/1000", view)
	}
}

// TestGetBillingPlanConvertsToCurrencyWithKnownRate is BACK-19's
// currency-conversion acceptance criterion: a non-USD currency with a
// BACK-11 rate gets a real converted amount, not the hardcoded USD figure.
func TestGetBillingPlanConvertsToCurrencyWithKnownRate(t *testing.T) {
	rates := newFakeExchangeRateRepo()
	currencies := newFakeCurrencyRepo("usd")
	if err := currencies.Add(context.Background(), "brl"); err != nil {
		t.Fatal(err)
	}
	// 1 usd = 5 brl, both 2-decimal currencies.
	if _, err := rates.Create(context.Background(), &dto.ExchangeRateDTO{
		UserID: "u1", Currency: "brl", UnitsPerUSD: "5", EffectiveFrom: time.Now().AddDate(0, 0, -1),
	}); err != nil {
		t.Fatal(err)
	}

	uc := NewGetBillingPlan(rates, currencies, 1000) // $10.00
	view, err := uc.Execute(context.Background(), "u1", "BRL")
	if err != nil {
		t.Fatal(err)
	}
	if view.Currency != "brl" {
		t.Errorf("Currency = %q, want brl (lowercased)", view.Currency)
	}
	if view.Amount != 5000 { // $10.00 * 5 = R$50.00 = 5000 cents
		t.Errorf("Amount = %d, want 5000 (R$50.00)", view.Amount)
	}
}

// TestGetBillingPlanFallsBackToUSDWithoutRate: never a hardcoded single
// currency's price presented as universal, but also never a wrong
// number — fall back to the documented reference currency.
func TestGetBillingPlanFallsBackToUSDWithoutRate(t *testing.T) {
	uc := NewGetBillingPlan(newFakeExchangeRateRepo(), newFakeCurrencyRepo(), 1000)
	view, err := uc.Execute(context.Background(), "u1", "jpy")
	if err != nil {
		t.Fatal(err)
	}
	if view.Currency != "usd" || view.Amount != 1000 {
		t.Errorf("view = %+v, want fallback usd/1000 when no rate exists", view)
	}
}
