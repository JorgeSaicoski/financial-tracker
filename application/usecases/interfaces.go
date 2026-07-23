// interfaces.go gathers every use-case contract of the application layer
// in one place: the *UseCase interfaces the handlers depend on, and the
// Input/Result types that cross the boundary with them. Everything is
// expressed in application/dto types and primitives — never domain
// entities, which stay inside usecase implementations and the domain
// layer. Implementations live in the sibling files, one per use case.
package usecases

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/dto"
)

// ---- Movements ----

// CreateMovementInput carries the caller-supplied fields for a single
// movement. Category and PaymentMethod default to "other" when empty so
// pre-existing clients that only send an amount keep working; both are
// validated against the domain's fixed lists inside the usecase.
type CreateMovementInput struct {
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	Category      string
	PaymentMethod string
	AccountID     *string
}

type CreateMovementUseCase interface {
	Execute(ctx context.Context, input CreateMovementInput) (*dto.MovementDTO, error)
}

type GetMovementUseCase interface {
	Execute(ctx context.Context, id string) (*dto.MovementDTO, error)
}

// ListMovementsResult also carries the computed balance, since
// ledger-service deliberately leaves that calculation to consumers.
type ListMovementsResult struct {
	Movements []*dto.MovementDTO
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
	Movement *dto.MovementDTO
	Reversal *dto.MovementDTO
}

type CancelMovementUseCase interface {
	Execute(ctx context.Context, id string) (CancelMovementResult, error)
}

// UpdateMovementInput carries a PATCH /movements/{id} partial body — a nil
// field means "leave unchanged". Description/Category/PaymentMethod/
// AccountID are metadata: local-only, always editable regardless of sync
// status. Amount/Currency/Timestamp are financial: editable in place only
// before the movement syncs; once synced, editing them produces a
// reversal + a replacement instead (see UpdateMovementResult).
type UpdateMovementInput struct {
	Description   *string
	Category      *string
	PaymentMethod *string
	AccountID     *string // a pointer to "" clears the account
	Amount        *int64
	Currency      *string
	Timestamp     *time.Time
}

// UpdateMovementResult reports how the edit was carried out. A
// metadata-only edit, or a financial edit on a not-yet-synced movement,
// updates Movement in place (Reversal/Replacement nil). A financial edit
// on an already-synced movement leaves Movement untouched other than the
// reversal link and returns the compensating Reversal plus the
// Replacement movement carrying the corrected values — mirroring
// CancelMovementResult's shape for the same reason (ledger-service never
// deletes).
type UpdateMovementResult struct {
	Movement    *dto.MovementDTO
	Reversal    *dto.MovementDTO
	Replacement *dto.MovementDTO
}

type UpdateMovementUseCase interface {
	Execute(ctx context.Context, id string, input UpdateMovementInput) (UpdateMovementResult, error)
}

// ---- Transfers ----

// TransferBetweenAccountsInput describes a move of money between two of
// the user's own accounts. Amount is always positive: the debit leg gets
// -Amount, the credit leg +Amount. A zero Timestamp means "now". v1 is
// same-currency only.
type TransferBetweenAccountsInput struct {
	UserID        string
	FromAccountID string
	ToAccountID   string
	Amount        int64
	Description   string
	Timestamp     time.Time
}

// TransferResult carries both legs of a transfer, linked by TransferID:
// Debit is the negative leg on FromAccountID, Credit the positive leg on
// ToAccountID. Together they net to zero, so the transfer never changes
// net worth.
type TransferResult struct {
	TransferID string
	Debit      *dto.MovementDTO
	Credit     *dto.MovementDTO
}

type TransferBetweenAccountsUseCase interface {
	Execute(ctx context.Context, input TransferBetweenAccountsInput) (TransferResult, error)
}

// CancelTransferResult reports what happened to each leg — same
// voided/reversal shape as CancelMovementResult, one per leg, since each
// leg is cancelled independently based on its own sync status.
type CancelTransferResult struct {
	Debit  CancelMovementResult
	Credit CancelMovementResult
}

type CancelTransferUseCase interface {
	Execute(ctx context.Context, transferID string) (CancelTransferResult, error)
}

// ---- Credit card purchases ----

type CreateCreditCardPurchaseInput struct {
	UserID       string
	TotalAmount  int64
	Currency     string
	Description  string
	Category     string
	Installments int
}

type CreateCreditCardPurchaseUseCase interface {
	Execute(ctx context.Context, input CreateCreditCardPurchaseInput) (*dto.CreditCardPurchaseDTO, []*dto.MovementDTO, error)
}

// CancelCreditCardPurchaseResult reports what happened to each
// installment: due/synced ones got reversals, not-yet-due ones were just
// voided (they never reached ledger-service).
type CancelCreditCardPurchaseResult struct {
	Purchase  *dto.CreditCardPurchaseDTO
	Voided    []*dto.MovementDTO
	Reversals []*dto.MovementDTO
}

type CancelCreditCardPurchaseUseCase interface {
	Execute(ctx context.Context, id string) (CancelCreditCardPurchaseResult, error)
}

// ---- Accounts ----

// CreateAccountInput carries the caller-supplied fields for a new
// account. Type defaults to "other" when empty and is validated against
// the domain's fixed list in the usecase; Currency must already be
// registered (POST /currencies first for a new one).
type CreateAccountInput struct {
	UserID   string
	Name     string
	Type     string
	Currency string
}

type CreateAccountUseCase interface {
	Execute(ctx context.Context, input CreateAccountInput) (*dto.AccountDTO, error)
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
	Account              *dto.AccountDTO
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

// ---- Exchange rates ----

// SetExchangeRateInput carries a POST /exchange-rates body. EffectiveFrom
// defaults to today (usecase fills it in when zero) and is normalized to
// midnight UTC — rates are dated, not timestamped. UnitsPerUSD is a
// decimal string ("5", "5.25"), never a float, so a value converted
// through it stays exact. Posting the same (UserID, Currency,
// EffectiveFrom) again replaces that row instead of creating a duplicate.
type SetExchangeRateInput struct {
	UserID        string
	Currency      string
	UnitsPerUSD   string
	EffectiveFrom time.Time
}

type SetExchangeRateUseCase interface {
	Execute(ctx context.Context, input SetExchangeRateInput) (*dto.ExchangeRateDTO, error)
}

// ExchangeRateGroup is one currency's current rate (nil if the user has
// never set one) plus its full history, newest EffectiveFrom first.
type ExchangeRateGroup struct {
	Currency string
	Current  *dto.ExchangeRateDTO
	History  []*dto.ExchangeRateDTO
}

// ListExchangeRatesUseCase groups the user's full rate history by
// currency, for GET /exchange-rates.
type ListExchangeRatesUseCase interface {
	Execute(ctx context.Context, userID string) ([]ExchangeRateGroup, error)
}

// DeleteExchangeRateUseCase removes a rate row the user owns — fixing a
// typo in history is legitimate, this is reference data, not movements.
type DeleteExchangeRateUseCase interface {
	Execute(ctx context.Context, userID, id string) error
}

// ToUSDUseCase converts amount (in currency's smallest unit) to USD's
// smallest unit (cents), using the rate effective at or before "at" — the
// row with the greatest EffectiveFrom <= at. Reused by BACK-12's
// purchasing-power report and future cross-currency transfers. Returns an
// error wrapping apperrors.ErrNotFound when no rate is known for currency
// at that time, so callers can surface that instead of a silently wrong
// number.
type ToUSDUseCase interface {
	Execute(ctx context.Context, userID string, amount int64, currency string, at time.Time) (int64, error)
}
