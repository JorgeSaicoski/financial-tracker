package dto

import (
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/domain/entities"
)

// CreditCardPurchaseDTO is the application layer's representation of an
// installment-purchase grouping record. Category and Status are plain
// strings, validated in usecases via the domain types.
type CreditCardPurchaseDTO struct {
	ID               string
	UserID           string
	Description      string
	Category         string
	TotalAmount      int64 // signed, smallest currency unit
	Currency         string
	InstallmentCount int
	PurchaseDate     time.Time
	Status           string
	CreatedAt        time.Time
}

func CreditCardPurchaseFromEntity(p *entities.CreditCardPurchase) *CreditCardPurchaseDTO {
	if p == nil {
		return nil
	}
	return &CreditCardPurchaseDTO{
		ID:               p.ID,
		UserID:           p.UserID,
		Description:      p.Description,
		Category:         string(p.Category),
		TotalAmount:      p.TotalAmount,
		Currency:         p.Currency,
		InstallmentCount: p.InstallmentCount,
		PurchaseDate:     p.PurchaseDate,
		Status:           string(p.Status),
		CreatedAt:        p.CreatedAt,
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
		Category:         entities.Category(p.Category),
		TotalAmount:      p.TotalAmount,
		Currency:         p.Currency,
		InstallmentCount: p.InstallmentCount,
		PurchaseDate:     p.PurchaseDate,
		Status:           entities.CreditCardPurchaseStatus(p.Status),
		CreatedAt:        p.CreatedAt,
	}
}
