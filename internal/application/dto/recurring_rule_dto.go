package dto

import (
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

// RecurringRuleDTO is the application layer's representation of a
// recurring movement rule. PaymentMethod is a plain string, validated
// against the domain's fixed list in usecases. CategoryID is the actual
// foreign key (BACK-14 follow-up) — Category is a read-only display
// name resolved by the repository via a join on read, same convention
// as MovementDTO.
type RecurringRuleDTO struct {
	ID            string
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	CategoryID    *string
	Category      string
	PaymentMethod string

	AccountID *string

	DayOfMonth string

	StartsAt time.Time
	EndsAt   *time.Time

	Active bool

	LastGeneratedAt *time.Time

	CreatedAt time.Time
}

func RecurringRuleFromEntity(r *entities.RecurringRule) *RecurringRuleDTO {
	if r == nil {
		return nil
	}
	return &RecurringRuleDTO{
		ID:              r.ID,
		UserID:          r.UserID,
		Amount:          r.Amount,
		Currency:        r.Currency,
		Description:     r.Description,
		CategoryID:      r.CategoryID,
		PaymentMethod:   string(r.PaymentMethod),
		AccountID:       r.AccountID,
		DayOfMonth:      r.DayOfMonth,
		StartsAt:        r.StartsAt,
		EndsAt:          r.EndsAt,
		Active:          r.Active,
		LastGeneratedAt: r.LastGeneratedAt,
		CreatedAt:       r.CreatedAt,
	}
}

func (r *RecurringRuleDTO) ToEntity() *entities.RecurringRule {
	if r == nil {
		return nil
	}
	return &entities.RecurringRule{
		ID:              r.ID,
		UserID:          r.UserID,
		Amount:          r.Amount,
		Currency:        r.Currency,
		Description:     r.Description,
		CategoryID:      r.CategoryID,
		PaymentMethod:   entities.PaymentMethod(r.PaymentMethod),
		AccountID:       r.AccountID,
		DayOfMonth:      r.DayOfMonth,
		StartsAt:        r.StartsAt,
		EndsAt:          r.EndsAt,
		Active:          r.Active,
		LastGeneratedAt: r.LastGeneratedAt,
		CreatedAt:       r.CreatedAt,
	}
}
