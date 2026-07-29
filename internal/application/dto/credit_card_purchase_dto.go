package dto

import (
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

// CreditCardPurchaseDTO is the application layer's representation of an
// installment-purchase grouping record. Status is a plain string,
// validated in usecases via the domain type. CategoryID is the actual
// foreign key (BACK-14 follow-up) — Category is a read-only display
// name resolved by the repository via a join on read, same convention
// as MovementDTO; see that type's own comment.
type CreditCardPurchaseDTO struct {
	ID               string
	UserID           string
	Description      string
	CategoryID       *string
	Category         string
	TotalAmount      int64 // signed, smallest currency unit
	Currency         string
	InstallmentCount int
	PurchaseDate     time.Time
	Status           string
	CreatedAt        time.Time

	CardID *string
}

func CreditCardPurchaseFromEntity(p *entities.CreditCardPurchase) *CreditCardPurchaseDTO {
	if p == nil {
		return nil
	}
	return &CreditCardPurchaseDTO{
		ID:               p.ID,
		UserID:           p.UserID,
		Description:      p.Description,
		CategoryID:       p.CategoryID,
		TotalAmount:      p.TotalAmount,
		Currency:         p.Currency,
		InstallmentCount: p.InstallmentCount,
		PurchaseDate:     p.PurchaseDate,
		Status:           string(p.Status),
		CreatedAt:        p.CreatedAt,
		CardID:           p.CardID,
	}
}

func (p *CreditCardPurchaseDTO) ToEntity() *entities.CreditCardPurchase {
	if p == nil {
		return nil
	}
	return &entities.CreditCardPurchase{
		ID:               p.ID,
		UserID:           p.UserID,
		Description:      p.Description,
		CategoryID:       p.CategoryID,
		TotalAmount:      p.TotalAmount,
		Currency:         p.Currency,
		InstallmentCount: p.InstallmentCount,
		PurchaseDate:     p.PurchaseDate,
		Status:           entities.CreditCardPurchaseStatus(p.Status),
		CreatedAt:        p.CreatedAt,
		CardID:           p.CardID,
	}
}
