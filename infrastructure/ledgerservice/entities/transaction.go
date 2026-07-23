// Package entities holds internal wire-format structs matching
// ledger-service's JSON payloads. They are private to the ledgerservice
// infrastructure package and convert to the application layer's DTOs via
// ToDTO() — "infrastructure adapts to the application's contract".
// Application and domain code never see these types.
package entities

import (
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
)

type Transaction struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Amount    int64     `json:"amount"`
	Currency  string    `json:"currency"`
	Timestamp time.Time `json:"timestamp"`
}

// ToDTO adapts ledger-service's transaction shape to the application
// layer's MovementDTO. Only the money facts exist on the wire, so the
// remaining DTO fields stay zero.
func (t Transaction) ToDTO() *dto.MovementDTO {
	return &dto.MovementDTO{
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
