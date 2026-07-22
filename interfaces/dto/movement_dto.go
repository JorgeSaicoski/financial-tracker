package dto

import "time"

// CreateMovementRequest is the API request body for POST /movements.
// UserID and Currency are optional: the handler fills in configured
// defaults when they're omitted. Description, Category and PaymentMethod
// are optional too (category/payment_method default to "other").
// Installments only matters when payment_method is "credit_card": a value
// above 1 splits the purchase into that many monthly movements.
type CreateMovementRequest struct {
	UserID        string `json:"user_id,omitempty"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency,omitempty"`
	Description   string `json:"description,omitempty"`
	Category      string `json:"category,omitempty"`
	PaymentMethod string `json:"payment_method,omitempty"`
	Installments  int    `json:"installments,omitempty"`
	AccountID     string `json:"account_id,omitempty"`
}

type MovementResponse struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Description   string    `json:"description,omitempty"`
	Category      string    `json:"category"`
	PaymentMethod string    `json:"payment_method"`
	Status        string    `json:"status"`
	SyncStatus    string    `json:"sync_status"`
	Timestamp     time.Time `json:"timestamp"`

	AccountID            string `json:"account_id,omitempty"`
	LedgerTransactionID  string `json:"ledger_transaction_id,omitempty"`
	CreditCardPurchaseID string `json:"credit_card_purchase_id,omitempty"`
	InstallmentNumber    int    `json:"installment_number,omitempty"`
	CancelsMovementID    string `json:"cancels_movement_id,omitempty"`
	ReversedByMovementID string `json:"reversed_by_movement_id,omitempty"`
}

type ListMovementsResponse struct {
	Movements []MovementResponse `json:"movements"`
	Balance   int64              `json:"balance"`
}

// CreditCardPurchaseResponse is returned by POST /movements when the
// request split a credit-card purchase into installments.
type CreditCardPurchaseResponse struct {
	ID               string             `json:"id"`
	UserID           string             `json:"user_id"`
	Description      string             `json:"description,omitempty"`
	Category         string             `json:"category"`
	TotalAmount      int64              `json:"total_amount"`
	Currency         string             `json:"currency"`
	InstallmentCount int                `json:"installment_count"`
	PurchaseDate     time.Time          `json:"purchase_date"`
	Status           string             `json:"status"`
	Movements        []MovementResponse `json:"movements,omitempty"`
}

// CancelMovementResponse: reversal is present only when the movement had
// already synced to ledger-service (immutable there, so it's compensated
// rather than voided).
type CancelMovementResponse struct {
	Movement MovementResponse  `json:"movement"`
	Reversal *MovementResponse `json:"reversal,omitempty"`
}

type CancelCreditCardPurchaseResponse struct {
	Purchase  CreditCardPurchaseResponse `json:"purchase"`
	Voided    []MovementResponse         `json:"voided"`
	Reversals []MovementResponse         `json:"reversals"`
}

type SyncSummaryResponse struct {
	Synced int `json:"synced"`
	Failed int `json:"failed"`
}

type CategoriesResponse struct {
	Categories     []string `json:"categories"`
	PaymentMethods []string `json:"payment_methods"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
