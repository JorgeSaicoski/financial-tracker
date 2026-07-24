package entities

// PaymentMethod is how a movement was paid. The set is fixed; validation
// happens in usecases (and is mirrored by a CHECK constraint in SQLite).
type PaymentMethod string

const (
	PaymentMethodCash         PaymentMethod = "cash"
	PaymentMethodDebitCard    PaymentMethod = "debit_card"
	PaymentMethodCreditCard   PaymentMethod = "credit_card"
	PaymentMethodPix          PaymentMethod = "pix"
	PaymentMethodBankTransfer PaymentMethod = "bank_transfer"
	PaymentMethodOther        PaymentMethod = "other"
)

func PaymentMethods() []PaymentMethod {
	return []PaymentMethod{
		PaymentMethodCash,
		PaymentMethodDebitCard,
		PaymentMethodCreditCard,
		PaymentMethodPix,
		PaymentMethodBankTransfer,
		PaymentMethodOther,
	}
}

func (p PaymentMethod) IsValid() bool {
	for _, m := range PaymentMethods() {
		if p == m {
			return true
		}
	}
	return false
}
