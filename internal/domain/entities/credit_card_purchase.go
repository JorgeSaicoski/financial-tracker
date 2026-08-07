package entities

import "time"

// CreditCardPurchase groups the installments of one credit-card purchase
// that was split over time. It is a grouping record only — the actual
// money movements are the Movement rows carrying its ID.
type CreditCardPurchase struct {
	ID          string
	UserID      string
	Description string
	// CategoryID references the shared categories registry (BACK-14
	// follow-up) — nil means genuinely uncategorized. See
	// Movement.CategoryID's comment for why there's no display name
	// carried alongside it here.
	CategoryID       *string
	TotalAmount      int64 // signed, smallest currency unit
	Currency         string
	InstallmentCount int
	PurchaseDate     time.Time
	Status           CreditCardPurchaseStatus
	CreatedAt        time.Time
}

type CreditCardPurchaseStatus string

const (
	CreditCardPurchaseStatusActive    CreditCardPurchaseStatus = "active"
	CreditCardPurchaseStatusCancelled CreditCardPurchaseStatus = "cancelled"
)
