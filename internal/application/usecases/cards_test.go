package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func seedCard(t *testing.T, cards *fakeCardRepo, closingDay, dueDay string) *dto.CardDTO {
	t.Helper()
	card, err := cards.Create(context.Background(), &dto.CardDTO{
		UserID: "u1", Name: "Visa", ClosingDay: closingDay, DueDay: dueDay, Currency: "usd",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return card
}

func TestCreateCardValidatesDay(t *testing.T) {
	uc := NewCreateCard(newFakeCardRepo())
	for _, bad := range []string{"", "0", "29", "31", "abc"} {
		_, err := uc.Execute(context.Background(), CreateCardInput{
			UserID: "u1", Name: "Visa", ClosingDay: bad, DueDay: "10", Currency: "usd",
		})
		if !errors.Is(err, apperrors.ErrInvalidInput) {
			t.Errorf("closing_day %q: want ErrInvalidInput, got %v", bad, err)
		}
	}
}

func TestCreateCardRejectsNegativeLimits(t *testing.T) {
	uc := NewCreateCard(newFakeCardRepo())
	negative := int64(-100)
	_, err := uc.Execute(context.Background(), CreateCardInput{
		UserID: "u1", Name: "Visa", ClosingDay: "5", DueDay: "15", Currency: "usd", CreditLimit: &negative,
	})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput for negative credit_limit, got %v", err)
	}
}

func TestListCardsZeroMovementsReportsZero(t *testing.T) {
	cards := newFakeCardRepo()
	movements := newFakeMovementRepo()
	seedCard(t, cards, "5", "15")

	views, err := NewListCards(cards, movements).Execute(context.Background(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("want 1 card, got %d", len(views))
	}
	if views[0].NextDueTotal != 0 || views[0].OpenCycleTotal != 0 {
		t.Errorf("want zero totals for a card with no movements, got %+v", views[0])
	}
}

func TestCardViewBucketsChargesByDueDate(t *testing.T) {
	cards := newFakeCardRepo()
	movements := newFakeMovementRepo()
	card := seedCard(t, cards, "5", "15")

	now := time.Now().UTC()
	nextDueDate := entities.CurrentStatementDueDate(card.ClosingDay, card.DueDay, now)
	afterNextDue := nextDueDate.AddDate(0, 1, 0)

	// A charge due exactly on the already-closed statement's due date.
	movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -1000, Currency: "usd", CardID: &card.ID,
		Status: string(entities.MovementStatusActive), Timestamp: nextDueDate, CreatedAt: now,
	})
	// A charge due on a later (still-open) cycle.
	movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -2500, Currency: "usd", CardID: &card.ID,
		Status: string(entities.MovementStatusActive), Timestamp: afterNextDue, CreatedAt: now,
	})

	view, err := NewGetCard(cards, movements).Execute(context.Background(), "u1", card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.NextDueTotal != 1000 {
		t.Errorf("NextDueTotal = %d, want 1000", view.NextDueTotal)
	}
	if view.OpenCycleTotal != 2500 {
		t.Errorf("OpenCycleTotal = %d, want 2500", view.OpenCycleTotal)
	}
}

func TestCardViewExcludesVoidedCharges(t *testing.T) {
	cards := newFakeCardRepo()
	movements := newFakeMovementRepo()
	card := seedCard(t, cards, "5", "15")

	now := time.Now().UTC()
	nextDueDate := entities.CurrentStatementDueDate(card.ClosingDay, card.DueDay, now)

	movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -1000, Currency: "usd", CardID: &card.ID,
		Status: string(entities.MovementStatusVoided), Timestamp: nextDueDate, CreatedAt: now,
	})

	view, err := NewGetCard(cards, movements).Execute(context.Background(), "u1", card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.NextDueTotal != 0 {
		t.Errorf("a voided charge should not count toward NextDueTotal, got %d", view.NextDueTotal)
	}
}

func TestCardPaymentReducesNextDueTotal(t *testing.T) {
	cards := newFakeCardRepo()
	movements := newFakeMovementRepo()
	card := seedCard(t, cards, "5", "15")

	now := time.Now().UTC()
	nextDueDate := entities.CurrentStatementDueDate(card.ClosingDay, card.DueDay, now)

	movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -1000, Currency: "usd", CardID: &card.ID,
		Status: string(entities.MovementStatusActive), Timestamp: nextDueDate, CreatedAt: now,
	})
	// A payment: negative expense movement on the paying account, marked
	// via CardPaymentForCardID rather than CardID.
	movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -400, Currency: "usd", CardPaymentForCardID: &card.ID,
		Status: string(entities.MovementStatusActive), Timestamp: now, CreatedAt: now,
	})

	view, err := NewGetCard(cards, movements).Execute(context.Background(), "u1", card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.NextDueTotal != 600 {
		t.Errorf("NextDueTotal = %d, want 600 (1000 charged - 400 paid)", view.NextDueTotal)
	}
	// Payments never affect the open (not-yet-due) cycle total.
	if view.OpenCycleTotal != 0 {
		t.Errorf("OpenCycleTotal = %d, want 0", view.OpenCycleTotal)
	}
}

func TestCardAvailableCreditAndBudget(t *testing.T) {
	cards := newFakeCardRepo()
	movements := newFakeMovementRepo()
	limit := int64(10000)
	budget := int64(3000)
	card, err := cards.Create(context.Background(), &dto.CardDTO{
		UserID: "u1", Name: "Visa", ClosingDay: "5", DueDay: "15", Currency: "usd",
		CreditLimit: &limit, MonthlyBudget: &budget, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	nextDueDate := entities.CurrentStatementDueDate(card.ClosingDay, card.DueDay, now)
	afterNextDue := nextDueDate.AddDate(0, 1, 0)

	movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -1000, Currency: "usd", CardID: &card.ID,
		Status: string(entities.MovementStatusActive), Timestamp: nextDueDate, CreatedAt: now,
	})
	movements.add(&dto.MovementDTO{
		UserID: "u1", Amount: -3500, Currency: "usd", CardID: &card.ID,
		Status: string(entities.MovementStatusActive), Timestamp: afterNextDue, CreatedAt: now,
	})

	view, err := NewGetCard(cards, movements).Execute(context.Background(), "u1", card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.AvailableCredit == nil || *view.AvailableCredit != 10000-1000-3500 {
		t.Errorf("AvailableCredit = %v, want %d", view.AvailableCredit, 10000-1000-3500)
	}
	if view.BudgetRemaining == nil || *view.BudgetRemaining != 3000-3500 {
		t.Errorf("BudgetRemaining = %v, want %d", view.BudgetRemaining, 3000-3500)
	}
	if !view.OverBudget {
		t.Error("want OverBudget=true when open-cycle spend exceeds monthly_budget")
	}
}

func TestCardAvailableCreditAndBudgetAbsentWhenUnset(t *testing.T) {
	cards := newFakeCardRepo()
	movements := newFakeMovementRepo()
	card := seedCard(t, cards, "5", "15")

	view, err := NewGetCard(cards, movements).Execute(context.Background(), "u1", card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.AvailableCredit != nil {
		t.Error("AvailableCredit should be nil when credit_limit is unset")
	}
	if view.BudgetRemaining != nil {
		t.Error("BudgetRemaining should be nil when monthly_budget is unset")
	}
}

func TestDeleteCardRejectsWhenReferenced(t *testing.T) {
	cards := newFakeCardRepo()
	card := seedCard(t, cards, "5", "15")
	cards.referenced[card.ID] = true

	err := NewDeleteCard(cards).Execute(context.Background(), "u1", card.ID)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("want ErrConflict for a referenced card, got %v", err)
	}
}

func TestDeleteCardSucceedsWhenUnused(t *testing.T) {
	cards := newFakeCardRepo()
	card := seedCard(t, cards, "5", "15")

	if err := NewDeleteCard(cards).Execute(context.Background(), "u1", card.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := cards.GetByID(context.Background(), "u1", card.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

func TestUpdateCardChangesFields(t *testing.T) {
	cards := newFakeCardRepo()
	card := seedCard(t, cards, "5", "15")
	uc := NewUpdateCard(cards)

	newName := "Renamed Card"
	newClosing := "10"
	updated, err := uc.Execute(context.Background(), "u1", card.ID, UpdateCardInput{
		Name: &newName, ClosingDay: &newClosing,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Renamed Card" || updated.ClosingDay != "10" {
		t.Errorf("update did not apply: %+v", updated)
	}
	if updated.DueDay != "15" {
		t.Errorf("unrelated field changed: due_day = %q", updated.DueDay)
	}
}

func TestUpdateCardRejectsInvalidDay(t *testing.T) {
	cards := newFakeCardRepo()
	card := seedCard(t, cards, "5", "15")
	bad := "31"

	_, err := NewUpdateCard(cards).Execute(context.Background(), "u1", card.ID, UpdateCardInput{ClosingDay: &bad})
	if !errors.Is(err, apperrors.ErrInvalidInput) {
		t.Errorf("want ErrInvalidInput, got %v", err)
	}
}
