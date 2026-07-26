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
	uc := NewUpdateRecurringRule(repo)

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
	uc := NewUpdateRecurringRule(repo)

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
	uc := NewUpdateRecurringRule(repo)

	bad := "31"
	if _, err := uc.Execute(context.Background(), "u1", rule.ID, UpdateRecurringRuleInput{DayOfMonth: &bad}); !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}

func TestUpdateRecurringRuleClearsAccount(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	accountID := "acc-1"
	rule, _ := repo.Create(context.Background(), &dto.RecurringRuleDTO{
		UserID: "u1", Amount: -1000, Currency: "usd", Category: "other", PaymentMethod: "other",
		AccountID: &accountID, DayOfMonth: "1", StartsAt: time.Now().UTC(), Active: true,
	})
	uc := NewUpdateRecurringRule(repo)

	empty := ""
	updated, err := uc.Execute(context.Background(), "u1", rule.ID, UpdateRecurringRuleInput{AccountID: &empty})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.AccountID != nil {
		t.Errorf("account_id should be cleared, got %v", *updated.AccountID)
	}
}

func TestUpdateRecurringRuleNotFound(t *testing.T) {
	uc := NewUpdateRecurringRule(newFakeRecurringRuleRepo())
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
	uc := NewUpdateRecurringRule(repo)

	active := false
	if _, err := uc.Execute(context.Background(), "someone-else", rule.ID, UpdateRecurringRuleInput{Active: &active}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound for another user's rule, got %v", err)
	}
}
