package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func seedRule(t *testing.T, repo *fakeRecurringRuleRepo) *dto.RecurringRuleDTO {
	t.Helper()
	rule, err := repo.Create(context.Background(), &dto.RecurringRuleDTO{
		UserID: "u1", Amount: -1000, Currency: "usd", Category: "housing", PaymentMethod: "bank_transfer",
		DayOfMonth: "1", StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Active: true,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func TestUpdateRecurringRuleDeactivatesWithoutTouchingOtherFields(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	rule := seedRule(t, repo)
	uc := NewUpdateRecurringRule(repo, newFakeAccountRepo())

	active := false
	updated, err := uc.Execute(context.Background(), "u1", rule.ID, UpdateRecurringRuleInput{Active: &active})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Active {
		t.Error("rule should be deactivated")
	}
	if updated.Amount != -1000 || updated.DayOfMonth != "1" {
		t.Errorf("deactivating changed unrelated fields: %+v", updated)
	}
}

func TestUpdateRecurringRuleEditsScheduleAndFinancialFields(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	rule := seedRule(t, repo)
	uc := NewUpdateRecurringRule(repo, newFakeAccountRepo())

	newAmount := int64(-2500)
	newDay := "15"
	updated, err := uc.Execute(context.Background(), "u1", rule.ID, UpdateRecurringRuleInput{
		Amount: &newAmount, DayOfMonth: &newDay,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Amount != newAmount || updated.DayOfMonth != newDay {
		t.Errorf("update did not apply: %+v", updated)
	}
}

func TestUpdateRecurringRuleRejectsInvalidDayOfMonth(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	rule := seedRule(t, repo)
	uc := NewUpdateRecurringRule(repo, newFakeAccountRepo())

	bad := "31"
	if _, err := uc.Execute(context.Background(), "u1", rule.ID, UpdateRecurringRuleInput{DayOfMonth: &bad}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

func TestUpdateRecurringRuleAcceptsEndsAtEqualToStartsAt(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	rule := seedRule(t, repo) // starts_at 2026-01-01
	uc := NewUpdateRecurringRule(repo, newFakeAccountRepo())

	sameDay := rule.StartsAt
	updated, err := uc.Execute(context.Background(), "u1", rule.ID, UpdateRecurringRuleInput{EndsAt: &sameDay})
	if err != nil {
		t.Fatalf("ends_at == starts_at should be a valid one-day rule, got error: %v", err)
	}
	if updated.EndsAt == nil || !updated.EndsAt.Equal(sameDay) {
		t.Errorf("want EndsAt %v, got %v", sameDay, updated.EndsAt)
	}
}

// TestUpdateRecurringRuleRejectsWithoutPartialWrite guards against a PATCH
// that fails validation on a later field (ends_at) after already writing
// an earlier one (amount/currency) — the whole request must apply or fail
// atomically from the caller's perspective, not leave the rule half-updated.
func TestUpdateRecurringRuleRejectsWithoutPartialWrite(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	rule := seedRule(t, repo) // starts_at 2026-01-01, amount -1000
	uc := NewUpdateRecurringRule(repo, newFakeAccountRepo())

	newAmount := int64(-9999)
	badEndsAt := rule.StartsAt.AddDate(0, 0, -1) // before starts_at: invalid
	_, err := uc.Execute(context.Background(), "u1", rule.ID, UpdateRecurringRuleInput{
		Amount: &newAmount, EndsAt: &badEndsAt,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}

	unchanged, err := repo.GetByID(context.Background(), rule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Amount != -1000 {
		t.Errorf("amount should be unchanged after a rejected PATCH, got %d", unchanged.Amount)
	}
}

func TestUpdateRecurringRuleClearsAccount(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	accountID := "acc-1"
	rule, _ := repo.Create(context.Background(), &dto.RecurringRuleDTO{
		UserID: "u1", Amount: -1000, Currency: "usd", Category: "other", PaymentMethod: "other",
		AccountID: &accountID, DayOfMonth: "1", StartsAt: time.Now().UTC(), Active: true,
	})
	uc := NewUpdateRecurringRule(repo, newFakeAccountRepo())

	empty := ""
	updated, err := uc.Execute(context.Background(), "u1", rule.ID, UpdateRecurringRuleInput{AccountID: &empty})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.AccountID != nil {
		t.Errorf("account_id should be cleared, got %v", *updated.AccountID)
	}
}

// TestUpdateRecurringRuleRejectsAccountCurrencyMismatch guards the gap the
// pr-second-opinion review caught: CreateRecurringRule validates
// account/rule currency match on the way in, but PATCH .../account_id could
// silently set a rule onto an account in a different currency, producing
// generated movements CreateMovement would reject on every future pass.
func TestUpdateRecurringRuleRejectsAccountCurrencyMismatch(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	rule := seedRule(t, repo) // currency "usd"
	accounts := newFakeAccountRepo()
	acc, _ := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "u1", Name: "Checking", Currency: "brl"})
	uc := NewUpdateRecurringRule(repo, accounts)

	if _, err := uc.Execute(context.Background(), "u1", rule.ID, UpdateRecurringRuleInput{AccountID: &acc.ID}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for currency mismatch, got %v", err)
	}
}

func TestUpdateRecurringRuleAcceptsMatchingAccountCurrency(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	rule := seedRule(t, repo) // currency "usd"
	accounts := newFakeAccountRepo()
	acc, _ := accounts.Create(context.Background(), &dto.AccountDTO{UserID: "u1", Name: "Checking", Currency: "usd"})
	uc := NewUpdateRecurringRule(repo, accounts)

	updated, err := uc.Execute(context.Background(), "u1", rule.ID, UpdateRecurringRuleInput{AccountID: &acc.ID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.AccountID == nil || *updated.AccountID != acc.ID {
		t.Errorf("want account_id %q, got %v", acc.ID, updated.AccountID)
	}
}

func TestUpdateRecurringRuleNotFound(t *testing.T) {
	uc := NewUpdateRecurringRule(newFakeRecurringRuleRepo(), newFakeAccountRepo())
	if _, err := uc.Execute(context.Background(), "u1", "missing", UpdateRecurringRuleInput{}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// TestUpdateRecurringRuleRejectsOtherUsersRule guards the IDOR this
// usecase used to allow: before the userID ownership check was added,
// any caller could edit or deactivate another user's rule just by
// guessing/enumerating its ID, since UpdateMetadata/SetActive etc. write
// straight to the row by id alone.
func TestUpdateRecurringRuleRejectsOtherUsersRule(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	rule := seedRule(t, repo) // owned by "u1"
	uc := NewUpdateRecurringRule(repo, newFakeAccountRepo())

	active := false
	if _, err := uc.Execute(context.Background(), "someone-else", rule.ID, UpdateRecurringRuleInput{Active: &active}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound for another user's rule, got %v", err)
	}
}
