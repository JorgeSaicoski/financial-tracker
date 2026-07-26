package dto

import (
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

// CardDTO is the application layer's representation of a card profile
// (BACK-08). CreditLimit/MonthlyBudget are independent optional fields —
// see entities.Card's doc comment.
type CardDTO struct {
	ID            string
	UserID        string
	Name          string
	LastFour      string
	ClosingDay    string
	DueDay        string
	CreditLimit   *int64
	MonthlyBudget *int64
	Currency      string
	CreatedAt     time.Time
}

func CardFromEntity(c *entities.Card) *CardDTO {
	if c == nil {
		return nil
	}
	return &CardDTO{
		ID:            c.ID,
		UserID:        c.UserID,
		Name:          c.Name,
		LastFour:      c.LastFour,
		ClosingDay:    c.ClosingDay,
		DueDay:        c.DueDay,
		CreditLimit:   c.CreditLimit,
		MonthlyBudget: c.MonthlyBudget,
		Currency:      c.Currency,
		CreatedAt:     c.CreatedAt,
	}
}

func (c *CardDTO) ToEntity() *entities.Card {
	if c == nil {
		return nil
	}
	return &entities.Card{
		ID:            c.ID,
		UserID:        c.UserID,
		Name:          c.Name,
		LastFour:      c.LastFour,
		ClosingDay:    c.ClosingDay,
		DueDay:        c.DueDay,
		CreditLimit:   c.CreditLimit,
		MonthlyBudget: c.MonthlyBudget,
		Currency:      c.Currency,
		CreatedAt:     c.CreatedAt,
	}
}
