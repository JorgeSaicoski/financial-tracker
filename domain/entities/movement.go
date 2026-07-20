package entities

import "time"

// Movement is a single financial movement (income or expense) for a user.
// It currently maps 1:1 to a ledger-service transaction: amount is stored
// in the smallest currency unit and its sign carries the direction, so
// there is no separate "type" field to keep in sync.
type Movement struct {
	ID        string
	UserID    string
	Amount    int64
	Currency  string
	Timestamp time.Time
}

func (m Movement) IsCredit() bool {
	return m.Amount > 0
}

func (m Movement) IsDebit() bool {
	return m.Amount < 0
}
