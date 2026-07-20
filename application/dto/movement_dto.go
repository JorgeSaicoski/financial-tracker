package dto

import "time"

// CreateMovementInput is what the application needs to record a movement.
// UserID and Currency are already resolved (defaults applied) by the time
// a usecase sees this — the API layer is responsible for that.
type CreateMovementInput struct {
	UserID   string
	Amount   int64
	Currency string
}

type MovementOutput struct {
	ID        string
	UserID    string
	Amount    int64
	Currency  string
	Timestamp time.Time
}

type ListMovementsInput struct {
	UserID   string
	Currency *string
	Limit    int
	Offset   int
}

type ListMovementsOutput struct {
	Movements []MovementOutput
	Balance   int64
}
