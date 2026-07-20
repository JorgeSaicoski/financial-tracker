package dto

import "time"

// CreateMovementRequest is the API request body for POST /movements.
// UserID and Currency are optional: the handler fills in configured
// defaults when they're omitted, so an MVP frontend with no auth/currency
// picker yet can just send an amount.
type CreateMovementRequest struct {
	UserID   string `json:"user_id,omitempty"`
	Amount   int64  `json:"amount"`
	Currency string `json:"currency,omitempty"`
}

type MovementResponse struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Amount    int64     `json:"amount"`
	Currency  string    `json:"currency"`
	Timestamp time.Time `json:"timestamp"`
}

type ListMovementsResponse struct {
	Movements []MovementResponse `json:"movements"`
	Balance   int64              `json:"balance"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
