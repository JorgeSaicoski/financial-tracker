package entities

// PaymentMethodCreditCard and PaymentMethodBankTransfer are the two
// payment-method names code branches on directly (movement_handler.go
// requires "credit_card" before accepting installments; Account.Send/
// Receive set "bank_transfer" on transfer legs) — kept as plain string
// constants so those comparisons/assignments still compile, even though
// payment methods themselves are no longer a fixed enum (BACK-17 turned
// them into a per-user registry, see
// application/repositories/payment_method_repository.go). Both are
// "system" entries: lazily ensured per user, not renamable or deletable
// through the payment-method API — a user renaming the string
// "credit_card" away would break their own installments.
const (
	PaymentMethodCreditCard   = "credit_card"
	PaymentMethodBankTransfer = "bank_transfer"
)
