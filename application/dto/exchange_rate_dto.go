package dto

import "time"

// ExchangeRateDTO is the application layer's representation of a
// historical exchange-rate row. Like currencies, exchange rates have no
// single-entity business rules worth a domain entity — this is the whole
// shape, application layer through infrastructure.
type ExchangeRateDTO struct {
	ID            string
	UserID        string
	Currency      string
	UnitsPerUSD   string // decimal string, e.g. "5" or "5.25" — never a float
	EffectiveFrom time.Time
	CreatedAt     time.Time
}
