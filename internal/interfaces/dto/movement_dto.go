package dto

import "time"

// CreateMovementRequest is the API request body for POST /movements.
// user_id is deliberately not a field here — BACK-02 derives it from the
// authenticated request's token, never from the body, so a client can't
// create a movement under another user's id. Currency is optional: the
// handler fills in the configured default when omitted. Description,
// Category and PaymentMethod are optional too (category/payment_method
// default to "other"). Installments only matters when payment_method is
// "credit_card": a value above 1 splits the purchase into that many
// monthly movements.
type CreateMovementRequest struct {
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency,omitempty"`
	Description   string `json:"description,omitempty"`
	Category      string `json:"category,omitempty"`
	PaymentMethod string `json:"payment_method,omitempty"`
	Installments  int    `json:"installments,omitempty"`
	AccountID     string `json:"account_id,omitempty"`
	// AvoidabilityOverridePercent (0-100, BACK-14) is this movement's own
	// ad-hoc avoidability, for a one-off spend that doesn't deserve its
	// own category — wins over the resolved category's own value.
	AvoidabilityOverridePercent *int `json:"avoidability_percent,omitempty"`
}

type MovementResponse struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Description   string    `json:"description,omitempty"`
	Category      string    `json:"category"`
	PaymentMethod string    `json:"payment_method"`
	Status        string    `json:"status"`
	SyncStatus    string    `json:"sync_status"`
	Timestamp     time.Time `json:"timestamp"`

	AccountID                   string `json:"account_id,omitempty"`
	LedgerTransactionID         string `json:"ledger_transaction_id,omitempty"`
	CreditCardPurchaseID        string `json:"credit_card_purchase_id,omitempty"`
	InstallmentNumber           int    `json:"installment_number,omitempty"`
	CancelsMovementID           string `json:"cancels_movement_id,omitempty"`
	ReversedByMovementID        string `json:"reversed_by_movement_id,omitempty"`
	TransferID                  string `json:"transfer_id,omitempty"`
	RecurringRuleID             string `json:"recurring_rule_id,omitempty"`
	AvoidabilityOverridePercent *int   `json:"avoidability_percent,omitempty"`
}

// UpdateMovementRequest is the API request body for PATCH /movements/{id}.
// A field absent from the JSON body leaves that value unchanged; an
// explicit "account_id": "" clears the account. Description, category,
// payment_method and account_id are metadata (always editable); amount,
// currency and timestamp are financial (see the UpdateMovement use case for
// what happens when they're edited on an already-synced movement).
type UpdateMovementRequest struct {
	Description   *string    `json:"description,omitempty"`
	Category      *string    `json:"category,omitempty"`
	PaymentMethod *string    `json:"payment_method,omitempty"`
	AccountID     *string    `json:"account_id,omitempty"`
	Amount        *int64     `json:"amount,omitempty"`
	Currency      *string    `json:"currency,omitempty"`
	Timestamp     *time.Time `json:"timestamp,omitempty"`
	// AvoidabilityOverridePercent (0-100, BACK-14): absent leaves it
	// unchanged, like every other field here.
	AvoidabilityOverridePercent *int `json:"avoidability_percent,omitempty"`
}

// UpdateMovementResponse: Reversal/Replacement are present only when the
// edit touched an already-synced movement's financial fields — the
// original (Movement) stayed as-is other than the reversal link, Reversal
// compensates it, and Replacement carries the corrected values.
type UpdateMovementResponse struct {
	Movement    MovementResponse  `json:"movement"`
	Reversal    *MovementResponse `json:"reversal,omitempty"`
	Replacement *MovementResponse `json:"replacement,omitempty"`
}

type ListMovementsResponse struct {
	Movements []MovementResponse `json:"movements"`
	Balance   int64              `json:"balance"`
}

// CreditCardPurchaseResponse is returned by POST /movements when the
// request split a credit-card purchase into installments.
type CreditCardPurchaseResponse struct {
	ID               string             `json:"id"`
	UserID           string             `json:"user_id"`
	Description      string             `json:"description,omitempty"`
	Category         string             `json:"category"`
	TotalAmount      int64              `json:"total_amount"`
	Currency         string             `json:"currency"`
	InstallmentCount int                `json:"installment_count"`
	PurchaseDate     time.Time          `json:"purchase_date"`
	Status           string             `json:"status"`
	Movements        []MovementResponse `json:"movements,omitempty"`
}

// CancelMovementResponse: reversal is present only when the movement had
// already synced to ledger-service (immutable there, so it's compensated
// rather than voided).
type CancelMovementResponse struct {
	Movement MovementResponse  `json:"movement"`
	Reversal *MovementResponse `json:"reversal,omitempty"`
}

type CancelCreditCardPurchaseResponse struct {
	Purchase  CreditCardPurchaseResponse `json:"purchase"`
	Voided    []MovementResponse         `json:"voided"`
	Reversals []MovementResponse         `json:"reversals"`
}

type SyncSummaryResponse struct {
	Synced int `json:"synced"`
	Failed int `json:"failed"`
}

// CategoryResponse is one row of the caller's category registry (BACK-14).
// AvoidabilityPercent is nil only for the two system categories
// ("transfer", "income") — no omitempty, so that comes across as an
// explicit JSON null a client can check for, not a missing key
// indistinguishable from a client/server version mismatch.
type CategoryResponse struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	AvoidabilityPercent *int   `json:"avoidability_percent"`
	// IsDefault (BACK-14 follow-up) marks the one category per user that
	// movements/purchases get reassigned to when their own category is
	// deleted.
	IsDefault bool `json:"is_default"`
}

type CategoriesResponse struct {
	Categories     []CategoryResponse `json:"categories"`
	PaymentMethods []string           `json:"payment_methods"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
