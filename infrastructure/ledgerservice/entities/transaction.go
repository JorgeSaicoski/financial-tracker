// Package entities holds internal wire-format structs matching
// ledger-service's JSON payloads. They are private to the ledgerservice
// infrastructure package and convert to the domain entity via ToEntity() -
// application and domain code never see these types.
package entities

import (
	"time"

	domain "github.com/JorgeSaicoski/financial-tracker/domain/entities"
)

type Transaction struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Amount    int64     `json:"amount"`
	Currency  string    `json:"currency"`
	Timestamp time.Time `json:"timestamp"`
}

func (t Transaction) ToEntity() *domain.Movement {
	return &domain.Movement{
		ID:        t.ID,
		UserID:    t.UserID,
		Amount:    t.Amount,
		Currency:  t.Currency,
		Timestamp: t.Timestamp,
	}
}

type TransactionRequest struct {
	UserID   string `json:"user_id"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type TransactionListResponse struct {
	Transactions []Transaction `json:"transactions"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
