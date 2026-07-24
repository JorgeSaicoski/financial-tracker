package dto

import "time"

// CreateAccountRequest is the API request body for POST /accounts.
// UserID defaults like movements; Type defaults to "other"; Currency must
// already be registered via POST /currencies.
type CreateAccountRequest struct {
	UserID   string `json:"user_id,omitempty"`
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Currency string `json:"currency,omitempty"`
}

// ReportBalanceRequest is the body for POST /accounts/{id}/balance: the
// balance the bank/broker/wallet actually shows right now, in the
// smallest currency unit. Pointer so an omitted balance is rejected
// rather than silently recorded as zero.
type ReportBalanceRequest struct {
	Balance *int64 `json:"balance"`
}

// AccountResponse is an account plus its derived balance picture.
// reported_* appear once the user has reported a real balance;
// last_return_* appear once there are two reports to compare, and expose
// how much the account yielded beyond the tracked movements.
type AccountResponse struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`

	EstimatedBalance     int64      `json:"estimated_balance"`
	MovementsSinceReport int64      `json:"movements_since_report"`
	ReportedBalance      *int64     `json:"reported_balance,omitempty"`
	ReportedAt           *time.Time `json:"reported_at,omitempty"`
	LastReturn           *int64     `json:"last_return,omitempty"`
	LastReturnFrom       *time.Time `json:"last_return_from,omitempty"`
	LastReturnTo         *time.Time `json:"last_return_to,omitempty"`
}

type AccountsResponse struct {
	Accounts     []AccountResponse `json:"accounts"`
	AccountTypes []string          `json:"account_types"`
}

type CurrenciesResponse struct {
	Currencies []string `json:"currencies"`
}

type AddCurrencyRequest struct {
	Code string `json:"code"`
}

type CashflowResponse struct {
	From      time.Time         `json:"from"`
	To        time.Time         `json:"to"`
	Totals    []CurrencyFlowDTO `json:"totals"`
	ByAccount []AccountFlowDTO  `json:"by_account"`
}

// CurrencyFlowDTO: in and out are both positive amounts; net = in - out.
type CurrencyFlowDTO struct {
	Currency string `json:"currency"`
	In       int64  `json:"in"`
	Out      int64  `json:"out"`
	Net      int64  `json:"net"`
}

// AccountFlowDTO: account_id/name are empty for movements not assigned
// to any account.
type AccountFlowDTO struct {
	AccountID string `json:"account_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Currency  string `json:"currency"`
	In        int64  `json:"in"`
	Out       int64  `json:"out"`
	Net       int64  `json:"net"`
}
