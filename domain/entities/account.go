package entities

import "time"

// Account is a place money sits (a bank account, a crypto wallet, an
// investment account). Each account holds exactly one currency; movements
// optionally reference an account so its balance can be tracked.
type Account struct {
	ID        string
	UserID    string
	Name      string
	Type      AccountType
	Currency  string
	CreatedAt time.Time
}

type AccountType string

const (
	AccountTypeBank       AccountType = "bank"
	AccountTypeInvestment AccountType = "investment"
	AccountTypeCrypto     AccountType = "crypto"
	AccountTypeCash       AccountType = "cash"
	AccountTypeOther      AccountType = "other"
)

func AccountTypes() []AccountType {
	return []AccountType{
		AccountTypeBank,
		AccountTypeInvestment,
		AccountTypeCrypto,
		AccountTypeCash,
		AccountTypeOther,
	}
}

func (t AccountType) IsValid() bool {
	for _, at := range AccountTypes() {
		if t == at {
			return true
		}
	}
	return false
}

// AccountSnapshot is a user-reported real balance at a point in time. We
// don't know an account's interest/yield up front; when the user reports
// what the account actually holds, the difference against the movements we
// tracked in between is the account's return.
type AccountSnapshot struct {
	ID        string
	AccountID string
	Balance   int64 // smallest currency unit, same convention as Movement.Amount
	Timestamp time.Time
	CreatedAt time.Time
}
