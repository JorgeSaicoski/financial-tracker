package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

func testCard() *dto.CardDTO {
	return &dto.CardDTO{
		UserID: "00000000-0000-0000-0000-000000000001", Name: "Visa", ClosingDay: "5", DueDay: "15",
		Currency: "usd", CreatedAt: time.Now().UTC(),
	}
}

func TestCardCreateGetRoundtrip(t *testing.T) {
	repo := NewCardRepository(openTestDB(t))
	ctx := context.Background()
	limit := int64(50000)

	card := testCard()
	card.CreditLimit = &limit
	created, err := repo.Create(ctx, card)
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, card.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Visa" || got.ClosingDay != "5" || got.DueDay != "15" ||
		got.CreditLimit == nil || *got.CreditLimit != 50000 || got.MonthlyBudget != nil {
		t.Errorf("got %+v", got)
	}
}

func TestCardGetByIDScopedToOwner(t *testing.T) {
	repo := NewCardRepository(openTestDB(t))
	ctx := context.Background()

	card := testCard()
	created, err := repo.Create(ctx, card)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByID(ctx, "someone-else", created.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound for another user's card, got %v", err)
	}
}

func TestCardListByUser(t *testing.T) {
	repo := NewCardRepository(openTestDB(t))
	ctx := context.Background()

	visa := testCard()
	visa.Name = "Visa"
	if _, err := repo.Create(ctx, visa); err != nil {
		t.Fatal(err)
	}
	other := testCard()
	other.UserID = "someone-else"
	other.Name = "Mastercard"
	if _, err := repo.Create(ctx, other); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListByUser(ctx, visa.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Visa" {
		t.Errorf("want exactly [Visa], got %+v", got)
	}
}

func TestCardUpdate(t *testing.T) {
	repo := NewCardRepository(openTestDB(t))
	ctx := context.Background()

	card := testCard()
	created, err := repo.Create(ctx, card)
	if err != nil {
		t.Fatal(err)
	}

	budget := int64(20000)
	update := &dto.CardDTO{Name: "Visa Renamed", LastFour: "4242", ClosingDay: "10", DueDay: "20", MonthlyBudget: &budget}
	if err := repo.Update(ctx, card.UserID, created.ID, update); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, card.UserID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Visa Renamed" || got.LastFour != "4242" || got.ClosingDay != "10" || got.DueDay != "20" ||
		got.MonthlyBudget == nil || *got.MonthlyBudget != 20000 {
		t.Errorf("got %+v", got)
	}
}

func TestCardDelete(t *testing.T) {
	repo := NewCardRepository(openTestDB(t))
	ctx := context.Background()

	card := testCard()
	created, err := repo.Create(ctx, card)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, card.UserID, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByID(ctx, card.UserID, created.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("want ErrNotFound after delete, got %v", err)
	}
}

func TestCardIsReferenced(t *testing.T) {
	db := openTestDB(t)
	cardRepo := NewCardRepository(db)
	movementRepo := NewMovementRepository(db)
	ctx := context.Background()

	card := testCard()
	created, err := cardRepo.Create(ctx, card)
	if err != nil {
		t.Fatal(err)
	}

	unused, err := cardRepo.IsReferenced(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unused {
		t.Error("a card with no movements should not be referenced")
	}

	m := testMovement(-500)
	m.CardID = &created.ID
	if _, err := movementRepo.Create(ctx, m); err != nil {
		t.Fatal(err)
	}

	referenced, err := cardRepo.IsReferenced(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !referenced {
		t.Error("a card with a linked movement should be referenced")
	}
}
