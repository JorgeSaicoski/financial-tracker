package usecases

import (
	"context"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

func TestListRecurringRulesFiltersByUser(t *testing.T) {
	repo := newFakeRecurringRuleRepo()
	repo.Create(context.Background(), &dto.RecurringRuleDTO{UserID: "u1", Amount: -1, Currency: "usd", DayOfMonth: "1", StartsAt: time.Now(), Active: true})
	repo.Create(context.Background(), &dto.RecurringRuleDTO{UserID: "u2", Amount: -1, Currency: "usd", DayOfMonth: "1", StartsAt: time.Now(), Active: true})

	uc := NewListRecurringRules(repo)
	rules, err := uc.Execute(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 || rules[0].UserID != "u1" {
		t.Errorf("rules = %+v, want exactly u1's one rule", rules)
	}
}
