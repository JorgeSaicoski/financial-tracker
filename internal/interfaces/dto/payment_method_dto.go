package dto

// CreatePaymentMethodRequest is the API request body for
// POST /payment-methods. Reserved system names ("credit_card",
// "bank_transfer") are rejected.
type CreatePaymentMethodRequest struct {
	Name string `json:"name"`
}

// UpdatePaymentMethodRequest is the API request body for
// PATCH /payment-methods/{id} — renaming is this endpoint's only
// capability; system entries reject any edit at all.
type UpdatePaymentMethodRequest struct {
	Name string `json:"name"`
}
