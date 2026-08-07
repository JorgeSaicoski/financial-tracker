package dto

import "time"

// CreateMovementRequest is the API request body for POST /movements.
// user_id is deliberately not a field here — BACK-02 derives it from the
// authenticated request's token, never from the body, so a client can't
// create a movement under another user's id. Currency is optional: the
// handler fills in the configured default when omitted. Description,
// CategoryID and PaymentMethod are optional too (payment_method defaults
// to "other"; an omitted category_id leaves the movement uncategorized).
// category_id (BACK-14 follow-up) is a real foreign key — it must
// reference an existing category, fetched via GET /categories first, not
// a free-text name. Installments only matters when payment_method is
// "credit_card": a value above 1 splits the purchase into that many
// monthly movements.
type CreateMovementRequest struct {
	Amount        int64   `json:"amount"`
	Currency      string  `json:"currency,omitempty"`
	Description   string  `json:"description,omitempty"`
	CategoryID    *string `json:"category_id,omitempty"`
	PaymentMethod string  `json:"payment_method,omitempty"`
	Installments  int     `json:"installments,omitempty"`
	AccountID     string  `json:"account_id,omitempty"`
	// PlanID (BACK-10) tags this movement as funding a savings plan.
	PlanID string `json:"plan_id,omitempty"`
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
	CategoryID    string    `json:"category_id,omitempty"`
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
	PlanID                      string `json:"plan_id,omitempty"`
	RecurringRuleID             string `json:"recurring_rule_id,omitempty"`
	AvoidabilityOverridePercent *int   `json:"avoidability_percent,omitempty"`
}

// UpdateMovementRequest is the API request body for PATCH /movements/{id}.
// A field absent from the JSON body leaves that value unchanged; an
// explicit "account_id": "" (or "category_id": "") clears it. Description,
// category_id, payment_method and account_id are metadata (always
// editable); amount, currency and timestamp are financial (see the
// UpdateMovement use case for what happens when they're edited on an
// already-synced movement).
type UpdateMovementRequest struct {
	Description   *string    `json:"description,omitempty"`
	CategoryID    *string    `json:"category_id,omitempty"`
	PaymentMethod *string    `json:"payment_method,omitempty"`
	AccountID     *string    `json:"account_id,omitempty"`
	PlanID        *string    `json:"plan_id,omitempty"`
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
	CategoryID       string             `json:"category_id,omitempty"`
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

// PaymentMethodResponse is one row of the caller's payment-method
// registry (BACK-17).
type PaymentMethodResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CategoryResponse is one row of the global category registry (BACK-14
// follow-up: categories are shared, not per-user — every caller sees the
// same list from GET /categories). AvoidabilityPercent is nil only for
// the three system categories ("transfer", "income", "other") — no
// omitempty, so that comes across as an explicit JSON null a client can
// check for, not a missing key indistinguishable from a client/server
// version mismatch.
type CategoryResponse struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	AvoidabilityPercent *int   `json:"avoidability_percent"`
	// IsContributor reports whether the requesting user may edit this
	// category (rename, change avoidability_percent) — computed relative
	// to whoever called GET /categories, so the frontend knows whether to
	// show edit controls without a second round-trip.
	IsContributor bool `json:"is_contributor"`
}

type CategoriesResponse struct {
	Categories     []CategoryResponse      `json:"categories"`
	PaymentMethods []PaymentMethodResponse `json:"payment_methods"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
