package dto

import "time"

// CreateRecurringRuleRequest is the API request body for
// POST /recurring-rules. UserID/Category/PaymentMethod default like
// CreateMovementRequest; DayOfMonth is required ("1"-"28" or "last").
// StartsAt omitted means "now"; EndsAt omitted means "no end date".
type CreateRecurringRuleRequest struct {
	UserID        string     `json:"user_id,omitempty"`
	Amount        int64      `json:"amount"`
	Currency      string     `json:"currency,omitempty"`
	Description   string     `json:"description,omitempty"`
	Category      string     `json:"category,omitempty"`
	PaymentMethod string     `json:"payment_method,omitempty"`
	AccountID     string     `json:"account_id,omitempty"`
	DayOfMonth    string     `json:"day_of_month"`
	StartsAt      *time.Time `json:"starts_at,omitempty"`
	EndsAt        *time.Time `json:"ends_at,omitempty"`
}

// UpdateRecurringRuleRequest is the API request body for
// PATCH /recurring-rules/{id}. A field absent from the JSON body leaves
// that value unchanged; an explicit "account_id": "" clears the account.
// There is no way to clear an already-set ends_at back to "no end date"
// through this endpoint (see the usecase's UpdateRecurringRuleInput
// comment) — a documented limitation.
type UpdateRecurringRuleRequest struct {
	Description   *string    `json:"description,omitempty"`
	Category      *string    `json:"category,omitempty"`
	PaymentMethod *string    `json:"payment_method,omitempty"`
	AccountID     *string    `json:"account_id,omitempty"`
	Amount        *int64     `json:"amount,omitempty"`
	Currency      *string    `json:"currency,omitempty"`
	DayOfMonth    *string    `json:"day_of_month,omitempty"`
	EndsAt        *time.Time `json:"ends_at,omitempty"`
	Active        *bool      `json:"active,omitempty"`
}

type RecurringRuleResponse struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	Amount          int64      `json:"amount"`
	Currency        string     `json:"currency"`
	Description     string     `json:"description,omitempty"`
	Category        string     `json:"category"`
	PaymentMethod   string     `json:"payment_method"`
	AccountID       string     `json:"account_id,omitempty"`
	DayOfMonth      string     `json:"day_of_month"`
	StartsAt        time.Time  `json:"starts_at"`
	EndsAt          *time.Time `json:"ends_at,omitempty"`
	Active          bool       `json:"active"`
	LastGeneratedAt *time.Time `json:"last_generated_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

type ListRecurringRulesResponse struct {
	RecurringRules []RecurringRuleResponse `json:"recurring_rules"`
}
