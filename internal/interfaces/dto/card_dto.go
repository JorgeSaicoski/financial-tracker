package dto

import "time"

// CreateCardRequest is the API request body for POST /cards. user_id is
// not a field here — like CreateMovementRequest, it comes from the
// authenticated request's token, never the body. closing_day/due_day
// must be "1"-"28" or "last"; credit_limit/monthly_budget are
// independent and both optional.
type CreateCardRequest struct {
	Name          string `json:"name"`
	LastFour      string `json:"last_four,omitempty"`
	ClosingDay    string `json:"closing_day"`
	DueDay        string `json:"due_day"`
	CreditLimit   *int64 `json:"credit_limit,omitempty"`
	MonthlyBudget *int64 `json:"monthly_budget,omitempty"`
	Currency      string `json:"currency"`
}

// UpdateCardRequest is the body for PATCH /cards/{id}. A field absent
// from the JSON body leaves that value unchanged. There's no way to
// clear an already-set credit_limit/monthly_budget back to "not tracked"
// through this endpoint (same documented limitation as
// PATCH /recurring-rules not being able to clear ends_at).
type UpdateCardRequest struct {
	Name          *string `json:"name,omitempty"`
	LastFour      *string `json:"last_four,omitempty"`
	ClosingDay    *string `json:"closing_day,omitempty"`
	DueDay        *string `json:"due_day,omitempty"`
	CreditLimit   *int64  `json:"credit_limit,omitempty"`
	MonthlyBudget *int64  `json:"monthly_budget,omitempty"`
}

// CardResponse is a card plus its computed amount-due/limit/budget
// picture. available_credit is present only when credit_limit is set;
// budget_remaining/over_budget only when monthly_budget is set.
type CardResponse struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Name          string    `json:"name"`
	LastFour      string    `json:"last_four,omitempty"`
	ClosingDay    string    `json:"closing_day"`
	DueDay        string    `json:"due_day"`
	CreditLimit   *int64    `json:"credit_limit,omitempty"`
	MonthlyBudget *int64    `json:"monthly_budget,omitempty"`
	Currency      string    `json:"currency"`
	CreatedAt     time.Time `json:"created_at"`

	NextDueTotal    int64     `json:"next_due_total"`
	NextDueDate     time.Time `json:"next_due_date"`
	OpenCycleTotal  int64     `json:"open_cycle_total"`
	AvailableCredit *int64    `json:"available_credit,omitempty"`
	BudgetRemaining *int64    `json:"budget_remaining,omitempty"`
	OverBudget      bool      `json:"over_budget,omitempty"`
}

type CardsResponse struct {
	Cards []CardResponse `json:"cards"`
}
