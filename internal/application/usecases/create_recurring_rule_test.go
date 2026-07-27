package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func TestCreateRecurringRuleValidatesDayOfMonth(t *testing.T) {
	uc := NewCreateRecurringRule(newFakeRecurringRuleRepo(), newFakeAccountRepo())

	for _, bad := range []string{"", "0", "29", "31", "last day", "abc"} {
		_, err := uc.Execute(context.Background(), CreateRecurringRuleInput{
			UserID: "u1", Amount: -100, Currency: "usd", DayOfMonth: bad,
		})
		if !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Errorf("day_of_month %q: want ErrInvalidInput, got %v", bad, err)
		}
	}
}

func TestCreateRecurringRuleAcceptsValidDayOfMonth(t *testing.T) {
	uc := NewCreateRecurringRule(newFakeRecurringRuleRepo(), newFakeAccountRepo())

	for _, good := range []string{"1", "28", "15", "last"} {
		rule, err := uc.Execute(context.Background(), CreateRecurringRuleInput{
			UserID: "u1", Amount: -100, Currency: "usd", DayOfMonth: good,
		})
		if err != nil {
			t.Errorf("day_of_month %q: unexpected error: %v", good, err)
			continue
		}
		if rule.DayOfMonth != good {
			t.Errorf("day_of_month %q: stored as %q", good, rule.DayOfMonth)
		}
		if !rule.Active {
			t.Errorf("day_of_month %q: new rule should default to active", good)
		}
	}
}

func TestCreateRecurringRuleRejectsAccountCurrencyMismatch(t *testing.T) {
	accounts := newFakeAccountRepo()
	acc, _ := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "u1", Name: "Checking", Currency: "brl"})
	uc := NewCreateRecurringRule(newFakeRecurringRuleRepo(), accounts)

	_, err := uc.Execute(context.Background(), CreateRecurringRuleInput{
		UserID: "u1", Amount: -100, Currency: "usd", DayOfMonth: "1", AccountID: &acc.ID,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for currency mismatch, got %v", err)
	}
}

func TestCreateRecurringRuleRejectsEndsAtBeforeStartsAt(t *testing.T) {
	uc := NewCreateRecurringRule(newFakeRecurringRuleRepo(), newFakeAccountRepo())
	starts := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	ends := starts.AddDate(0, 0, -1)

	_, err := uc.Execute(context.Background(), CreateRecurringRuleInput{
		UserID: "u1", Amount: -100, Currency: "usd", DayOfMonth: "1", StartsAt: starts, EndsAt: &ends,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for ends_at before starts_at, got %v", err)
	}
}

func TestCreateRecurringRuleAcceptsEndsAtEqualToStartsAt(t *testing.T) {
	uc := NewCreateRecurringRule(newFakeRecurringRuleRepo(), newFakeAccountRepo())
	starts := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	ends := starts

	rule, err := uc.Execute(context.Background(), CreateRecurringRuleInput{
		UserID: "u1", Amount: -100, Currency: "usd", DayOfMonth: "1", StartsAt: starts, EndsAt: &ends,
	})
	if err != nil {
		t.Fatalf("ends_at == starts_at should be a valid one-day rule, got error: %v", err)
	}
	if rule.EndsAt == nil || !rule.EndsAt.Equal(starts) {
		t.Errorf("want EndsAt %v, got %v", starts, rule.EndsAt)
	}
}

func TestCreateRecurringRuleValidatesEndsAtAgainstDefaultedStartsAt(t *testing.T) {
	uc := NewCreateRecurringRule(newFakeRecurringRuleRepo(), newFakeAccountRepo())
	// starts_at omitted (defaults to today) — ends_at in the past must
	// still be rejected against the *defaulted* value, not the zero time.
	yesterday := time.Now().UTC().AddDate(0, 0, -1)

	_, err := uc.Execute(context.Background(), CreateRecurringRuleInput{
		UserID: "u1", Amount: -100, Currency: "usd", DayOfMonth: "1", EndsAt: &yesterday,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for ends_at before defaulted starts_at, got %v", err)
	}
}

func TestCreateRecurringRuleDefaultsStartsAtToNow(t *testing.T) {
	uc := NewCreateRecurringRule(newFakeRecurringRuleRepo(), newFakeAccountRepo())

	before := time.Now().UTC()
	rule, err := uc.Execute(context.Background(), CreateRecurringRuleInput{
		UserID: "u1", Amount: -100, Currency: "usd", DayOfMonth: "1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule.StartsAt.Before(before.Truncate(24*time.Hour)) || rule.StartsAt.After(before.Add(24*time.Hour)) {
		t.Errorf("starts_at = %s, want close to now", rule.StartsAt)
	}
}
