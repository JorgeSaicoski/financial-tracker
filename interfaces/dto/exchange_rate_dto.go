package dto

import "time"

// SetExchangeRateRequest is the body for POST /exchange-rates.
// EffectiveFrom defaults to today when omitted. UnitsPerUSD is a decimal
// string ("5", "5.25"), never a JSON number, so precision survives the
// wire.
type SetExchangeRateRequest struct {
	Currency      string     `json:"currency"`
	UnitsPerUSD   string     `json:"units_per_usd"`
	EffectiveFrom *time.Time `json:"effective_from,omitempty"`
}

// ExchangeRateResponse is one historical rate row.
type ExchangeRateResponse struct {
	ID            string    `json:"id"`
	Currency      string    `json:"currency"`
	UnitsPerUSD   string    `json:"units_per_usd"`
	EffectiveFrom time.Time `json:"effective_from"`
	CreatedAt     time.Time `json:"created_at"`
}

// ExchangeRateGroupResponse is one currency's current rate (null if never
// set) plus its full history, newest first.
type ExchangeRateGroupResponse struct {
	Currency string                 `json:"currency"`
	Current  *ExchangeRateResponse  `json:"current"`
	History  []ExchangeRateResponse `json:"history"`
}

type ExchangeRatesResponse struct {
	Rates []ExchangeRateGroupResponse `json:"rates"`
}
