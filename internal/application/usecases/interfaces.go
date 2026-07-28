package usecases

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
)

// This file consolidates every use-case contract (interface plus its
// Input/Result/View types) in one place, per this workspace's
// CleanExampleGo-derived architecture rules — see AGENTS.md
// ("Architecture — follow CleanExampleGo"). Implementations live one per
// file, alongside their unexported struct and constructor.

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

// EnsureUserInput carries what the identity provider asserted about the
// caller (services.Identity, unpacked so this package doesn't depend on
// infrastructure/authentik). Called once per request by the auth
// middleware right after token verification succeeds — never hit
// directly over HTTP. Idempotent: the first sight of UserID inserts a
// row, every sight after refreshes the profile fields.
type EnsureUserInput struct {
	UserID      string
	Provider    string
	ExternalID  string
	Email       string
	DisplayName string
}

type EnsureUserUseCase interface {
	Execute(ctx context.Context, input EnsureUserInput) (*dto.UserDTO, error)
}

// GetUserUseCase backs GET /me. By the time any handler runs, the auth
// middleware's EnsureUser call has already provisioned the row, so
// ErrNotFound here would only mean a bug in that wiring, not a normal
// "new user" case.
type GetUserUseCase interface {
	Execute(ctx context.Context, userID string) (*dto.UserDTO, error)
}

type CreateCreditCardPurchaseInput struct {
	UserID       string
	TotalAmount  int64
	Currency     string
	Description  string
	Category     string
	Installments int
	// CardID (BACK-08), when set, dates each installment on the card's
	// real due days (closing_day/due_day) instead of the flat monthly
	// offset from the purchase date.
	CardID *string
}

type CreateCreditCardPurchaseUseCase interface {
	Execute(ctx context.Context, input CreateCreditCardPurchaseInput) (*dto.CreditCardPurchaseDTO, []*dto.MovementDTO, error)
}

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
	// CardID (BACK-08), accepted only when PaymentMethod is
	// "credit_card", dates this (single, non-installment) charge on the
	// card's real due day instead of leaving Timestamp as "now".
	CardID *string
	// CardPaymentForCardID (BACK-08) marks this movement as a payment
	// settling the named card's statement — reduces that card's
	// next_due_total. Independent of CardID: a movement is either a
	// charge (CardID) or a payment (CardPaymentForCardID), never both.
	CardPaymentForCardID *string
}

// ImportRowInput is one CSV data row (BACK-03), still raw strings — the
// usecase does all type coercion/validation itself, applying the same
// rules CreateMovement enforces (category/payment-method validity,
// account currency match) field by field so a bad row reports exactly
// which field is wrong.
type ImportRowInput struct {
	Date          string
	Amount        string
	Currency      string
	Description   string
	Category      string
	PaymentMethod string
	Account       string // optional; resolved case-insensitively against the user's accounts
}

// ImportMovementsInput is one CSV file's rows plus the import mode
// flags.
type ImportMovementsInput struct {
	UserID         string
	Rows           []ImportRowInput
	DryRun         bool // validate (and report) only; never writes
	AllowPartial   bool // false (default/strict): any row error aborts the whole file
	SkipDuplicates bool // exclude flagged duplicate rows from the commit
}

// ImportRowError is one row's validation failure. Row is 1-based,
// counting only data rows (the row immediately after the header is row 1).
type ImportRowError struct {
	Row     int
	Field   string
	Message string
}

// ImportDuplicate flags a row that matches on (user, date, amount,
// currency, normalized description) either an already-existing movement
// (ExistingMovementID set) or an earlier row in the same file
// (DuplicateOfRow set, 1-based like Row). Duplicates still import by
// default; ImportMovementsInput.SkipDuplicates excludes them.
type ImportDuplicate struct {
	Row                int
	ExistingMovementID *string
	DuplicateOfRow     *int
}

// ImportMovementsResult mirrors POST /import/movements's response.
// Imported/Skipped describe what happened (a real commit) or what would
// happen (DryRun) — either way, the same two counts.
type ImportMovementsResult struct {
	Imported   int
	Skipped    int
	Errors     []ImportRowError
	Duplicates []ImportDuplicate
}

type ImportMovementsUseCase interface {
	Execute(ctx context.Context, input ImportMovementsInput) (ImportMovementsResult, error)
}

// ExportedRow is one CSV row of GET /export/movements — the first seven
// fields are exactly BACK-03's import model (Account already resolved to
// its name, not id), so export -> import round-trips. Status/
// CancelsMovementID/ReversedByMovementID are only populated (and only
// rendered by the handler) when the caller asked for IncludeCancelled.
type ExportedRow struct {
	Date          string // YYYY-MM-DD
	Amount        int64
	Currency      string
	Description   string
	Category      string
	PaymentMethod string
	Account       string

	Status               string
	CancelsMovementID    string
	ReversedByMovementID string
}

// ExportMovementsUseCase implements BACK-09's GET /export/movements: a
// CSV export in the exact BACK-03 import model, available in every mode
// (not just standalone) so a user's data is always portable. Default
// (includeCancelled=false) excludes voided movements and any movement
// that is a reversal or has been reversed — an export meant for re-import
// should read like the user's real history, not ledger-service's
// append-only correction trail.
type ExportMovementsUseCase interface {
	Execute(ctx context.Context, userID string, includeCancelled bool) ([]ExportedRow, error)
}

type CreateMovementUseCase interface {
	Execute(ctx context.Context, input CreateMovementInput) (*dto.MovementDTO, error)
}

// GetMovementUseCase requires the caller's userID and returns
// apperrors.ErrNotFound (not a distinguishable "forbidden") when the
// movement exists but belongs to someone else — the API must not leak
// whether an id exists at all to a caller who doesn't own it (BACK-02).
type GetMovementUseCase interface {
	Execute(ctx context.Context, userID, id string) (*dto.MovementDTO, error)
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

// UpdateMovementUseCase requires the caller's userID and returns
// apperrors.ErrNotFound when id belongs to someone else (BACK-02) — same
// ownership rule as GetMovementUseCase.
type UpdateMovementUseCase interface {
	Execute(ctx context.Context, userID, id string, input UpdateMovementInput) (UpdateMovementResult, error)
}

// CancelMovementResult reports how the cancel was carried out: a
// never-synced movement is voided in place (Reversal is nil); a synced one
// stays active and gains a compensating Reversal, mirroring
// ledger-service's no-delete rule.
type CancelMovementResult struct {
	Movement *dto.MovementDTO
	Reversal *dto.MovementDTO
}

// CancelMovementUseCase requires the caller's userID and returns
// apperrors.ErrNotFound when id belongs to someone else (BACK-02) — same
// ownership rule as GetMovementUseCase.
type CancelMovementUseCase interface {
	Execute(ctx context.Context, userID, id string) (CancelMovementResult, error)
}

// CancelCreditCardPurchaseResult reports what happened to each
// installment: due/synced ones got reversals, not-yet-due ones were just
// voided (they never reached ledger-service).
type CancelCreditCardPurchaseResult struct {
	Purchase  *dto.CreditCardPurchaseDTO
	Voided    []*dto.MovementDTO
	Reversals []*dto.MovementDTO
}

// CancelCreditCardPurchaseUseCase requires the caller's userID and
// returns apperrors.ErrNotFound when id belongs to someone else
// (BACK-02) — same ownership rule as GetMovementUseCase.
type CancelCreditCardPurchaseUseCase interface {
	Execute(ctx context.Context, userID, id string) (CancelCreditCardPurchaseResult, error)
}

// CancelTransferResult reports what happened to each leg — same
// voided/reversal shape as CancelMovementResult, one per leg, since each
// leg is cancelled independently based on its own sync status.
type CancelTransferResult struct {
	Debit  CancelMovementResult
	Credit CancelMovementResult
}

// CancelTransferUseCase requires the caller's userID and returns
// apperrors.ErrNotFound when transferID belongs to someone else
// (BACK-02) — same ownership rule as GetMovementUseCase.
type CancelTransferUseCase interface {
	Execute(ctx context.Context, userID, transferID string) (CancelTransferResult, error)
}

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
//
// Shared by ListAccountsUseCase and ReportAccountBalanceUseCase — both
// return the same shape via buildAccountView (list_accounts.go).
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
// exposes the account's return since the previous report. A zero
// timestamp means "now" — same convention as
// TransferBetweenAccountsInput.Timestamp — letting a caller backfill a
// report for an earlier date instead of always today. Requires the
// caller's userID and returns apperrors.ErrNotFound when accountID
// belongs to someone else (BACK-02) — same ownership rule as
// GetMovementUseCase.
type ReportAccountBalanceUseCase interface {
	Execute(ctx context.Context, userID, accountID string, balance int64, timestamp time.Time) (AccountView, error)
}

// AccountSnapshotView is one reported snapshot plus its own computed
// return: the balance change the movements since the previous snapshot
// (or, for the earliest snapshot, since the account's implicit zero
// starting balance) don't explain. Nil Return would only happen if
// NetByAccount itself failed, which the usecase turns into an error
// instead — so in practice every entry always carries one.
type AccountSnapshotView struct {
	Snapshot   *dto.AccountSnapshotDTO
	Return     int64
	ReturnFrom *time.Time // nil for the earliest snapshot (no previous one)
	ReturnTo   time.Time
}

// ListAccountSnapshotsUseCase returns one account's full reported-balance
// history, newest first, each entry paired with its own return — the
// per-snapshot generalization of AccountView's single LastReturn (which
// only ever covers the latest two). Requires the caller's userID and
// returns apperrors.ErrNotFound when accountID belongs to someone else
// (BACK-02) — same ownership rule as GetMovementUseCase.
type ListAccountSnapshotsUseCase interface {
	Execute(ctx context.Context, userID, accountID string) ([]AccountSnapshotView, error)
}

type ListCurrenciesUseCase interface {
	Execute(ctx context.Context) ([]string, error)
}

// AddCurrencyUseCase registers a new currency code; adding an existing
// code is a no-op. Returns the normalized (lowercased) code.
type AddCurrencyUseCase interface {
	Execute(ctx context.Context, code string) (string, error)
}

// UserSettingsView is what GET/PATCH /settings return (BACK-13).
// Entitled fields are operator/billing-controlled and read-only through
// this API; Enabled fields are user preference. Effective capability is
// Entitled AND Enabled.
type UserSettingsView struct {
	UserID               string
	LedgerSyncEntitled   bool
	LedgerSyncEnabled    bool
	CloudStorageEntitled bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// GetUserSettingsUseCase returns the caller's own settings, defaulting
// to "everything true" if they've never touched them.
type GetUserSettingsUseCase interface {
	Execute(ctx context.Context, userID string) (UserSettingsView, error)
}

// UpdateUserSettingsUseCase changes ledger_sync_enabled — the only field
// a user (as opposed to an operator) may write; entitlement fields are
// never accepted here (rejected at the HTTP decode boundary, see
// interfaces/dto). Toggling sync back on reclassifies the backlog
// accumulated while it was off (SyncStatusLocal) back to "pending" so
// the next sync pass picks it up.
type UpdateUserSettingsUseCase interface {
	Execute(ctx context.Context, userID string, ledgerSyncEnabled bool) (UserSettingsView, error)
}

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

// ArchiveBundle is the full restorable account state BACK-15's "no cloud"
// archive needs: everything ExportArchiveUseCase gathers and
// ImportArchiveUseCase consumes. Categories aren't included — they're
// still the fixed, hardcoded list (BACK-14 hasn't landed), so there's
// nothing user-defined to restore there yet; once BACK-14 lands, this
// bundle should grow a Categories field too.
type ArchiveBundle struct {
	Accounts            []*dto.AccountDTO
	Movements           []*dto.MovementDTO
	CreditCardPurchases []*dto.CreditCardPurchaseDTO
}

type ExportArchiveUseCase interface {
	Execute(ctx context.Context, userID string) (ArchiveBundle, error)
}

// ImportArchiveResult reports what a restore actually did. Restoring a
// row whose ID already exists is a no-op ("skipped"), not an error — so
// importing the same archive twice, or one that partially overlaps what's
// already there, is safe. Restore is a disaster-recovery path, not a
// merge tool: it never overwrites an existing row with the archive's copy.
type ImportArchiveResult struct {
	AccountsRestored, AccountsSkipped                       int
	MovementsRestored, MovementsSkipped                     int
	CreditCardPurchasesRestored, CreditCardPurchasesSkipped int
}

type ImportArchiveUseCase interface {
	Execute(ctx context.Context, userID string, bundle ArchiveBundle) (ImportArchiveResult, error)
}

// GetLocalArchiveSettingUseCase reads BACK-15's local_archive_enabled
// toggle, defaulting to false for a user who never set it.
type GetLocalArchiveSettingUseCase interface {
	Execute(ctx context.Context, userID string) (bool, error)
}

// SetLocalArchiveSettingUseCase flips the toggle. It never touches
// cloud_storage_enabled (BACK-16) or any other setting — the two are
// independent.
type SetLocalArchiveSettingUseCase interface {
	Execute(ctx context.Context, userID string, enabled bool) (bool, error)
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

// CreateRecurringRuleInput carries the caller-supplied fields for a new
// recurring rule (BACK-07). Category/PaymentMethod default to "other"
// like CreateMovementInput; DayOfMonth must be "1".."28" or "last".
// StartsAt zero means "now".
type CreateRecurringRuleInput struct {
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	Category      string
	PaymentMethod string
	AccountID     *string
	DayOfMonth    string
	StartsAt      time.Time
	EndsAt        *time.Time
}

type CreateRecurringRuleUseCase interface {
	Execute(ctx context.Context, input CreateRecurringRuleInput) (*dto.RecurringRuleDTO, error)
}

type ListRecurringRulesUseCase interface {
	Execute(ctx context.Context, userID string) ([]*dto.RecurringRuleDTO, error)
}

// UpdateRecurringRuleInput carries a PATCH /recurring-rules/{id} partial
// body — a nil field means "leave unchanged", mirroring
// UpdateMovementInput. Editing any field only affects future
// generations; movements already generated are never retroactively
// touched. EndsAt has no way to clear an existing end date back to "no
// end" via this input (nil means "don't change it", not "clear it") — a
// known, documented limitation rather than an oversight; the common case
// (setting or changing an end date) works normally.
type UpdateRecurringRuleInput struct {
	Description   *string
	Category      *string
	PaymentMethod *string
	AccountID     *string // a pointer to "" clears the account
	Amount        *int64
	Currency      *string
	DayOfMonth    *string
	EndsAt        *time.Time
	Active        *bool
}

// UpdateRecurringRuleUseCase requires the caller's userID and returns
// apperrors.ErrNotFound (not a distinguishable "forbidden") when the rule
// exists but belongs to someone else — same contract as GetMovementUseCase.
type UpdateRecurringRuleUseCase interface {
	Execute(ctx context.Context, userID, id string, input UpdateRecurringRuleInput) (*dto.RecurringRuleDTO, error)
}

// CreateCardInput carries the caller-supplied fields for a new card
// profile (BACK-08). ClosingDay/DueDay must be "1"-"28" or "last".
// CreditLimit/MonthlyBudget are independent and both optional — see
// entities.Card's doc comment for why.
type CreateCardInput struct {
	UserID        string
	Name          string
	LastFour      string
	ClosingDay    string
	DueDay        string
	CreditLimit   *int64
	MonthlyBudget *int64
	Currency      string
}

type CreateCardUseCase interface {
	Execute(ctx context.Context, input CreateCardInput) (*dto.CardDTO, error)
}

// UpdateCardInput carries a PATCH /cards/{id} partial body — a nil field
// means "leave unchanged", same convention as UpdateMovementInput.
// CreditLimit/MonthlyBudget can be set or changed but not cleared back to
// "not tracked" through this endpoint once set — same documented
// limitation as PATCH /recurring-rules not being able to clear ends_at.
type UpdateCardInput struct {
	Name          *string
	LastFour      *string
	ClosingDay    *string
	DueDay        *string
	CreditLimit   *int64
	MonthlyBudget *int64
}

type UpdateCardUseCase interface {
	Execute(ctx context.Context, userID, id string, input UpdateCardInput) (*dto.CardDTO, error)
}

type DeleteCardUseCase interface {
	Execute(ctx context.Context, userID, id string) error
}

type GetCardUseCase interface {
	Execute(ctx context.Context, userID, id string) (CardView, error)
}

type ListCardsUseCase interface {
	Execute(ctx context.Context, userID string) ([]CardView, error)
}

// CardView is a card plus its computed amount-due/limit/budget picture
// (BACK-08):
//
//   - NextDueTotal/NextDueDate: the statement that has already closed
//     and is waiting to be paid, net of payments already recorded
//     against this card.
//   - OpenCycleTotal: purchases made after the last closing day, still
//     accumulating toward the next statement — not due yet.
//   - AvailableCredit: CreditLimit minus everything outstanding
//     (NextDueTotal + OpenCycleTotal), only when CreditLimit is set.
//   - BudgetRemaining/OverBudget: MonthlyBudget minus OpenCycleTotal,
//     only when MonthlyBudget is set.
type CardView struct {
	Card            *dto.CardDTO
	NextDueTotal    int64
	NextDueDate     time.Time
	OpenCycleTotal  int64
	AvailableCredit *int64
	BudgetRemaining *int64
	OverBudget      bool
}
