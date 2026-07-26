package entities

import "time"

// CreditCardPurchase groups the installments of one credit-card purchase
// that was split over time. It is a grouping record only — the actual
// money movements are the Movement rows carrying its ID.
type CreditCardPurchase struct {
	ID               string
	UserID           string
	Description      string
	Category         Category
	TotalAmount      int64 // signed, smallest currency unit
	Currency         string
	InstallmentCount int
	PurchaseDate     time.Time
	Status           CreditCardPurchaseStatus
	CreatedAt        time.Time

	// CardID (BACK-08) links this purchase to the card it was made on —
	// nil keeps today's flat-offset installment date behavior unchanged.
	// Propagated onto each installment Movement's own CardID too, so
	// per-card aggregates only need to query movements.
	CardID *string
}

type CreditCardPurchaseStatus string

const (
	CreditCardPurchaseStatusActive    CreditCardPurchaseStatus = "active"
	CreditCardPurchaseStatusCancelled CreditCardPurchaseStatus = "cancelled"
)
