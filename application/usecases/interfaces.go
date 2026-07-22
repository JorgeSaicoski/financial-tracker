// interfaces.go gathers every use-case contract of the application layer
// in one place: the *UseCase interfaces the handlers depend on, and the
// Input/Result types that cross the boundary with them. Implementations
// live in the sibling files, one per use case.
package usecases

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/domain/entities"
)

// ---- Movements ----

// CreateMovementInput carries the caller-supplied fields for a single
// movement. Category and PaymentMethod default to "other" when empty so
// pre-existing clients that only send an amount keep working.
type CreateMovementInput struct {
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	Category      entities.Category
	PaymentMethod entities.PaymentMethod
	AccountID     *string
}

type CreateMovementUseCase interface {
	Execute(ctx context.Context, input CreateMovementInput) (*entities.Movement, error)
}

type GetMovementUseCase interface {
	Execute(ctx context.Context, id string) (*entities.Movement, error)
}

// ListMovementsResult also carries the computed balance, since
// ledger-service deliberately leaves that calculation to consumers.
type ListMovementsResult struct {
	Movements []*entities.Movement
	Balance   int64
}

type ListMovementsUseCase interface {
	Execute(ctx context.Context, userID string, currency *string, from, to *time.Time, limit, offset int) (ListMovementsResult, error)
}

// CancelMovementResult reports how the cancel was carried out: a
// never-synced movement is voided in place (Reversal is nil); a synced one
// stays active and gains a compensating Reversal, mirroring
// ledger-service's no-delete rule.
type CancelMovementResult struct {
	Movement *entities.Movement
	Reversal *entities.Movement
}

type CancelMovementUseCase interface {
	Execute(ctx context.Context, id string) (CancelMovementResult, error)
}

// ---- Credit card purchases ----

type CreateCreditCardPurchaseInput struct {
	UserID       string
	TotalAmount  int64
	Currency     string
	Description  string
	Category     entities.Category
	Installments int
}

type CreateCreditCardPurchaseUseCase interface {
	Execute(ctx context.Context, input CreateCreditCardPurchaseInput) (*entities.CreditCardPurchase, []*entities.Movement, error)
}

// CancelCreditCardPurchaseResult reports what happened to each
// installment: due/synced ones got reversals, not-yet-due ones were just
// voided (they never reached ledger-service).
type CancelCreditCardPurchaseResult struct {
	Purchase  *entities.CreditCardPurchase
	Voided    []*entities.Movement
	Reversals []*entities.Movement
}

type CancelCreditCardPurchaseUseCase interface {
	Execute(ctx context.Context, id string) (CancelCreditCardPurchaseResult, error)
}

// ---- Accounts ----

// CreateAccountInput carries the caller-supplied fields for a new
// account. Type defaults to "other" when empty; Currency must already be
// registered (POST /currencies first for a new one).
type CreateAccountInput struct {
	UserID   string
	Name     string
	Type     entities.AccountType
	Currency string
}

type CreateAccountUseCase interface {
	Execute(ctx context.Context, input CreateAccountInput) (*entities.Account, error)
}

// AccountView is an account plus everything derivable about its money:
//
//   - ReportedBalance/ReportedAt: the latest user-reported snapshot (nil
//     until the user reports one).
//   - MovementsSinceReport: net of tracked movements after that snapshot
//     (or all-time when there's no snapshot yet).
//   - EstimatedBalance: reported + movements since — our best guess of
//     what the account holds right now.
//   - LastReturn: between the last two snapshots, how much the balance
//     changed beyond what movements explain — the account's yield or
//     interest over LastReturnFrom..LastReturnTo. Needs two snapshots.
type AccountView struct {
	Account              *entities.Account
	EstimatedBalance     int64
	ReportedBalance      *int64
	ReportedAt           *time.Time
	MovementsSinceReport int64
	LastReturn           *int64
	LastReturnFrom       *time.Time
	LastReturnTo         *time.Time
}

type ListAccountsUseCase interface {
	Execute(ctx context.Context, userID string) ([]AccountView, error)
}

// ReportAccountBalanceUseCase records what the account really holds right
// now (per the bank/broker/wallet), as a snapshot. The returned view then
// exposes the account's return since the previous report.
type ReportAccountBalanceUseCase interface {
	Execute(ctx context.Context, accountID string, balance int64) (AccountView, error)
}

// ---- Cashflow ----

// CurrencyFlow aggregates the interval's money in / money out for one
// currency. In and Out are both positive; Net = In - Out. Totals are kept
// per currency because summing usd and btc together is meaningless.
type CurrencyFlow struct {
	Currency string
	In       int64
	Out      int64
	Net      int64
}

// AccountFlow is the same breakdown for one account. AccountID/Name are
// empty for movements that weren't assigned to any account.
type AccountFlow struct {
	AccountID string
	Name      string
	Currency  string
	In        int64
	Out       int64
	Net       int64
}

type CashflowResult struct {
	From      time.Time
	To        time.Time
	Totals    []CurrencyFlow
	ByAccount []AccountFlow
}

type GetCashflowUseCase interface {
	Execute(ctx context.Context, userID string, from, to time.Time) (CashflowResult, error)
}

// ---- Currencies ----

type ListCurrenciesUseCase interface {
	Execute(ctx context.Context) ([]string, error)
}

// AddCurrencyUseCase registers a new currency code; adding an existing
// code is a no-op. Returns the normalized (lowercased) code.
type AddCurrencyUseCase interface {
	Execute(ctx context.Context, code string) (string, error)
}
