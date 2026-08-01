package dto

import "time"

// CreateTransferRequest is the API request body for POST /transfers. v1 is
// same-currency only: FromAccountID and ToAccountID must reference
// accounts holding the same currency. Timestamp defaults to now when
// omitted. user_id is not a field here — like CreateMovementRequest, it
// comes from the authenticated request's token (BACK-02).
type CreateTransferRequest struct {
	FromAccountID string     `json:"from_account_id"`
	ToAccountID   string     `json:"to_account_id"`
	Amount        int64      `json:"amount"`
	Description   string     `json:"description,omitempty"`
	Timestamp     *time.Time `json:"timestamp,omitempty"`
	// PlanID (BACK-10), when set, tags the credit (destination) leg as
	// funding a savings plan — the recommended way to fund one.
	PlanID string `json:"plan_id,omitempty"`
}

// TransferResponse links both legs of a transfer by the shared
// TransferID: Debit is the negative leg on the source account, Credit the
// positive leg on the destination — together they net to zero.
type TransferResponse struct {
	TransferID string           `json:"transfer_id"`
	Debit      MovementResponse `json:"debit"`
	Credit     MovementResponse `json:"credit"`
}

// CancelTransferResponse reports what happened to each leg — same
// voided/reversal shape as CancelMovementResponse, one per leg, since
// each leg is cancelled independently based on its own sync status.
type CancelTransferResponse struct {
	Debit  CancelMovementResponse `json:"debit"`
	Credit CancelMovementResponse `json:"credit"`
}
