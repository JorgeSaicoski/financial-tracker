package entities

import (
	"errors"
	"fmt"
	"time"
)

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

// Send validates the transfer from this account's side and returns the
// debit leg (a negative Movement) to persist. It does not touch
// repositories or persist anything itself — that's the usecase's job,
// as is linking the pair via TransferID.
//
// Deliberately thin today: just the contract (same currency, positive
// amount, not sending to itself). Room to grow without changing the
// call site — e.g. a sufficient-balance check once Account tracks a
// balance, or an observability/monitoring hook here once we care about
// per-account transfer volume.
func (a *Account) Send(to *Account, amount int64, description string, timestamp time.Time) (*Movement, error) {
	if err := a.validateTransfer(to, amount); err != nil {
		return nil, err
	}
	return a.transferLeg(-amount, description, timestamp), nil
}

// Receive is Send's mirror for the destination side — same validation,
// the credit leg. Kept as its own method (not derived from Send) so each
// side can grow independently: e.g. a "did the target actually receive
// it" confirmation/monitoring hook later belongs here, not on the source
// account's method.
func (a *Account) Receive(from *Account, amount int64, description string, timestamp time.Time) (*Movement, error) {
	if err := a.validateTransfer(from, amount); err != nil {
		return nil, err
	}
	return a.transferLeg(amount, description, timestamp), nil
}

func (a *Account) validateTransfer(other *Account, amount int64) error {
	if other == nil {
		return errors.New("other account is required")
	}
	if a.ID != "" && other.ID != "" && a.ID == other.ID {
		return errors.New("cannot transfer to the same account")
	}
	if a.Currency != other.Currency {
		return fmt.Errorf("cross-currency transfers aren't supported yet (%q vs %q)", a.Currency, other.Currency)
	}
	if amount <= 0 {
		return errors.New("amount must be positive")
	}
	return nil
}

func (a *Account) transferLeg(amount int64, description string, timestamp time.Time) *Movement {
	return &Movement{
		UserID:        a.UserID,
		Amount:        amount,
		Currency:      a.Currency,
		Description:   description,
		Category:      CategoryTransfer,
		PaymentMethod: PaymentMethodBankTransfer,
		AccountID:     &a.ID,
		Status:        MovementStatusActive,
		SyncStatus:    SyncStatusPending,
		Timestamp:     timestamp,
		CreatedAt:     time.Now().UTC(),
	}
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
