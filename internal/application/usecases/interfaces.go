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
	CategoryID   *string
	Installments int
}

type CreateCreditCardPurchaseUseCase interface {
	Execute(ctx context.Context, input CreateCreditCardPurchaseInput) (*dto.CreditCardPurchaseDTO, []*dto.MovementDTO, error)
}

// CreateMovementInput carries the caller-supplied fields for a single
// movement. PaymentMethod defaults to "other" when empty, validated
// against the domain's fixed list inside the usecase. CategoryID (BACK-14
// follow-up) is a real foreign key now — nil leaves the movement
// uncategorized; a non-nil value must reference an existing category
// (any category, since they're globally shared — no ownership check).
type CreateMovementInput struct {
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	CategoryID    *string
	PaymentMethod string
	AccountID     *string
	// PlanID (BACK-10) tags this movement as funding a savings plan. Must
	// belong to the user, be an active savings plan, and match the
	// movement's currency — validated in the usecase.
	PlanID *string
	// AvoidabilityOverridePercent (0-100, BACK-14) is this movement's own
	// ad-hoc avoidability, for a one-off spend that doesn't deserve its
	// own category. Wins over the resolved category's avoidability_percent.
	AvoidabilityOverridePercent *int
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
// field means "leave unchanged". Description/CategoryID/PaymentMethod/
// AccountID are metadata: local-only, always editable regardless of sync
// status. Amount/Currency/Timestamp are financial: editable in place only
// before the movement syncs; once synced, editing them produces a
// reversal + a replacement instead (see UpdateMovementResult).
type UpdateMovementInput struct {
	Description *string
	// CategoryID: nil leaves it unchanged; a pointer to "" clears it
	// (genuinely uncategorized), same convention as AccountID.
	CategoryID    *string
	PaymentMethod *string
	AccountID     *string // a pointer to "" clears the account
	PlanID        *string // a pointer to "" clears the plan link (BACK-10)
	Amount        *int64
	Currency      *string
	Timestamp     *time.Time
	// AvoidabilityOverridePercent (0-100, BACK-14), like every other
	// field here: nil means "leave unchanged". Local-only metadata (same
	// group as Description/CategoryID/PaymentMethod/AccountID) — editable
	// regardless of sync status, no reversal/replacement produced.
	AvoidabilityOverridePercent *int
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
	// PlanID (BACK-10), when set, tags the credit (destination) leg only
	// — the recommended way to fund a savings plan without inflating
	// income/expense cashflow. Must belong to the user, be an active
	// savings plan, and match ToAccountID's currency.
	PlanID *string
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
// exposes the account's return since the previous report. Requires the
// caller's userID and returns apperrors.ErrNotFound when accountID
// belongs to someone else (BACK-02) — same ownership rule as
// GetMovementUseCase.
type ReportAccountBalanceUseCase interface {
	Execute(ctx context.Context, userID, accountID string, balance int64) (AccountView, error)
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
// Entitled AND Enabled. SubscriptionStatus/SubscriptionCurrentPeriodEnd
// (BACK-19) are the zero value/nil when the caller has never had a
// subscription row — a free-tier user is not an error case.
type UserSettingsView struct {
	UserID               string
	LedgerSyncEntitled   bool
	LedgerSyncEnabled    bool
	CloudStorageEntitled bool

	// DefaultCategoryID (BACK-14 follow-up): nil when the user has never
	// set one, which resolves to the global "other" category.
	DefaultCategoryID *string

	SubscriptionStatus           string
	SubscriptionCurrentPeriodEnd *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// GetUserSettingsUseCase returns the caller's own settings, defaulting
// to "everything true" if they've never touched them.
type GetUserSettingsUseCase interface {
	Execute(ctx context.Context, userID string) (UserSettingsView, error)
}

// UpdateUserSettingsInput carries a PATCH /settings partial body — nil
// means "leave unchanged" for each field independently, same convention
// as UpdateMovementInput. DefaultCategoryID's pointer-to-"" clears it
// back to the global "other" fallback.
type UpdateUserSettingsInput struct {
	LedgerSyncEnabled *bool
	DefaultCategoryID *string
}

// UpdateUserSettingsUseCase changes ledger_sync_enabled and/or
// default_category_id — the only fields a user (as opposed to an
// operator) may write; entitlement fields are never accepted here
// (rejected at the HTTP decode boundary, see interfaces/dto). Toggling
// sync back on reclassifies the backlog accumulated while it was off
// (SyncStatusLocal) back to "pending" so the next sync pass picks it up.
type UpdateUserSettingsUseCase interface {
	Execute(ctx context.Context, userID string, input UpdateUserSettingsInput) (UserSettingsView, error)
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

// CreatePaymentMethodInput carries a POST /payment-methods body (BACK-17).
type CreatePaymentMethodInput struct {
	UserID string
	Name   string
}

// CreatePaymentMethodUseCase rejects the two reserved system names
// ("credit_card", "bank_transfer") and duplicate names (case-insensitive).
type CreatePaymentMethodUseCase interface {
	Execute(ctx context.Context, input CreatePaymentMethodInput) (*dto.PaymentMethodDTO, error)
}

// ListPaymentMethodsUseCase lazily ensures the system entries and the
// first-run defaults exist for userID first (absence-safe, same pattern
// as user_settings), then returns every payment-method row, name
// ascending.
type ListPaymentMethodsUseCase interface {
	Execute(ctx context.Context, userID string) ([]*dto.PaymentMethodDTO, error)
}

// UpdatePaymentMethodInput carries a PATCH /payment-methods/{id} body —
// unlike UpdateMovementInput's nil-means-unchanged fields, Name here is
// always required: renaming is this endpoint's only capability.
type UpdatePaymentMethodInput struct {
	Name string
}

// UpdatePaymentMethodUseCase rejects edits to the two reserved system
// names and renaming onto an existing name (case-insensitive).
type UpdatePaymentMethodUseCase interface {
	Execute(ctx context.Context, userID, id string, input UpdatePaymentMethodInput) (*dto.PaymentMethodDTO, error)
}

// DeletePaymentMethodUseCase rejects deleting either reserved system
// name. Deleting one still referenced by movements is allowed — it's a
// label, not an FK, same as a deleted category.
type DeletePaymentMethodUseCase interface {
	Execute(ctx context.Context, userID, id string) error
}

// ProcessBillingWebhookInput carries what the payment provider's webhook
// asserts about one subscription (BACK-19), already translated from
// whatever the provider's own payload shape is (see
// services.PaymentWebhookVerifier's doc comment) into this
// provider-agnostic form.
type ProcessBillingWebhookInput struct {
	UserID                 string
	Provider               string
	ProviderSubscriptionID string
	Status                 string // dto.SubscriptionStatus*
	CurrentPeriodEnd       time.Time
}

// ProcessBillingWebhookUseCase upserts the subscription row and flips
// cloud_storage_entitled to true immediately for active — gaining access
// should never wait. Neither canceled nor past_due flips entitlement
// here at all: BACK-19's acceptance criteria are explicit that even an
// explicit cancellation "flips it back after the grace period, not
// immediately." The grace-period sweep (internal/application/billing) is
// what eventually flips either status to false once its grace period
// elapses.
type ProcessBillingWebhookUseCase interface {
	Execute(ctx context.Context, input ProcessBillingWebhookInput) (*dto.SubscriptionDTO, error)
}

// BillingPlanView is what GET /billing/plan returns: Currency is the
// currency Amount is actually expressed in — the caller's requested
// currency when a BACK-11 rate exists for it, or the reference currency
// ("usd") as a documented fallback otherwise. Never assume Currency ==
// what was requested; a client must read it back.
type BillingPlanView struct {
	Currency string
	Amount   int64 // Currency's smallest unit
}

// GetBillingPlanUseCase converts the fixed USD reference price to the
// caller's requested currency using their own BACK-11 exchange rate,
// falling back to the USD reference price when no rate is known for that
// currency (never a hardcoded USD string presented as universal).
type GetBillingPlanUseCase interface {
	Execute(ctx context.Context, userID, currency string) (BillingPlanView, error)
}

// CreatePlanInput carries a POST /plans body (BACK-10). TargetAmount and
// AccountID are required for a savings plan, and must be absent for a
// stress-test plan (a hypothetical cost has no real target or funding
// account). StartDate defaults to now when zero.
type CreatePlanInput struct {
	UserID              string
	Name                string
	Type                string
	TargetAmount        *int64
	Currency            string
	MonthlyTargetAmount int64
	AccountID           *string
	StartDate           time.Time
	EndDate             *time.Time
}

type CreatePlanUseCase interface {
	Execute(ctx context.Context, input CreatePlanInput) (*dto.PlanDTO, error)
}

// PlanSummary is GET /plans' lightweight per-plan progress: Progress is
// either a savings plan's all-time SUM(amount) toward TargetAmount, or a
// stress-test plan's current (not projected) month-to-date surplus —
// income minus expense minus MonthlyTargetAmount. It deliberately omits
// the pace checker's forward-looking fields (see PlanDetail) since
// listing every plan shouldn't pay for N cashflow scans just to render a
// summary row.
type PlanSummary struct {
	Plan          *dto.PlanDTO
	Progress      int64
	TargetReached bool
}

type ListPlansUseCase interface {
	Execute(ctx context.Context, userID string, at time.Time) ([]PlanSummary, error)
}

// PlanDetail is GET /plans/{id}'s full response: PlanSummary plus the
// pace checker. OnTrack: for a stress-test plan, whether the *projected*
// month-end surplus (month-to-date expense extrapolated linearly to
// month-end, income never extrapolated — it's lumpy, see the ticket's
// own v1 heuristic note) stays non-negative; for a savings plan, whether
// this month's contributions-to-date are at or ahead of linear pace.
// ProjectedShortfall is savings-only (month's MonthlyTargetAmount minus
// the projected end-of-month contribution total at the current rate) —
// nil for a stress-test plan.
type PlanDetail struct {
	PlanSummary
	OnTrack            bool
	ProjectedShortfall *int64
}

type GetPlanUseCase interface {
	Execute(ctx context.Context, userID, id string, at time.Time) (PlanDetail, error)
}

// UpdatePlanInput carries a PATCH /plans/{id} body — nil means "leave
// unchanged", same convention as UpdateMovementInput. Type, Currency, and
// AccountID are not editable after creation (changing them would silently
// invalidate every movement already tagged with this plan's id).
type UpdatePlanInput struct {
	Name                *string
	TargetAmount        *int64
	MonthlyTargetAmount *int64
	EndDate             *time.Time
	Status              *string
}

type UpdatePlanUseCase interface {
	Execute(ctx context.Context, userID, id string, input UpdatePlanInput) (*dto.PlanDTO, error)
}

// CreateCategoryInput carries a POST /categories body. AvoidabilityPercent
// nil defaults to a neutral 50 (same default the one-time migration
// backfill used) — the user edits it afterward. UserID becomes the new
// category's sole contributor (BACK-14 follow-up: categories are shared/
// global, not per-user — see entities.Category).
type CreateCategoryInput struct {
	UserID              string
	Name                string
	AvoidabilityPercent *int
}

// CreateCategoryUseCase rejects the three reserved system names
// ("transfer", "income", "other") and enforces the per-user
// max_categories_per_user limit (see LimitsRepository) — duplicate names
// are allowed: two different users' "restaurant" are two different rows.
type CreateCategoryUseCase interface {
	Execute(ctx context.Context, input CreateCategoryInput) (*dto.CategoryDTO, error)
}

// ListCategoriesUseCase returns every category in the system (BACK-14
// follow-up: categories are globally visible, not per-user), name
// ascending.
type ListCategoriesUseCase interface {
	Execute(ctx context.Context) ([]*dto.CategoryDTO, error)
}

// UpdateCategoryInput carries a PATCH /categories/{id} body — nil means
// "leave unchanged", same convention as UpdateMovementInput.
type UpdateCategoryInput struct {
	Name                *string
	AvoidabilityPercent *int
}

// UpdateCategoryUseCase rejects edits to the three reserved system
// names, an AvoidabilityPercent outside 0-100, and any edit from a
// caller who isn't one of the category's contributors (see
// entities.Category.CanBeEditedBy).
type UpdateCategoryUseCase interface {
	Execute(ctx context.Context, userID, id string, input UpdateCategoryInput) (*dto.CategoryDTO, error)
}

// DeleteCategoryUseCase implements DELETE /categories/{id}: there is no
// real delete once a category may be shared (BACK-14 follow-up) — this
// removes id from the caller's own category list only (see
// CategoryRepository.Remove), never touching the row itself or any other
// user's data. Existing movements/purchases/recurring rules already
// referencing id keep doing so; only future selection is blocked (see
// resolveCategoryID). reassignExisting, when true, also moves every
// movement/purchase/recurring rule the caller owns off id and onto their
// resolved default category first (CategoryRepository.RemoveAndReassign)
// — still scoped strictly to the caller's own rows. Rejects removing a
// reserved system category.
type DeleteCategoryUseCase interface {
	Execute(ctx context.Context, userID, id string, reassignExisting bool) error
}

// CreateRecurringRuleInput carries the caller-supplied fields for a new
// recurring rule (BACK-07). PaymentMethod defaults to "other" when empty.
// CategoryID (BACK-14 follow-up) is a real foreign key — nil stays
// uncategorized, a non-nil value must reference an existing category.
// DayOfMonth must be "1".."28" or "last". StartsAt zero means "now".
type CreateRecurringRuleInput struct {
	UserID        string
	Amount        int64
	Currency      string
	Description   string
	CategoryID    *string
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
	Description *string
	// CategoryID: nil leaves it unchanged; a pointer to "" clears it,
	// same convention as UpdateMovementInput.CategoryID.
	CategoryID    *string
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
