package handlers

import "net/http"

// This file consolidates every handler contract in one place, per this
// workspace's CleanExampleGo-derived architecture rules — see AGENTS.md
// ("Architecture — follow CleanExampleGo"). Implementations live one per
// file, alongside their unexported struct and constructor.

// AccountHandler exposes the accounts API: the places money sits, plus
// user-reported balances that let us compute each account's return.
type AccountHandler interface {
	CreateAccount(w http.ResponseWriter, r *http.Request)
	ListAccounts(w http.ResponseWriter, r *http.Request)
	ReportBalance(w http.ResponseWriter, r *http.Request)
	ListSnapshots(w http.ResponseWriter, r *http.Request)
}

// CardHandler exposes the cards API (BACK-08): credit-card profiles
// (closing/due day, optional limit/budget) plus each card's computed
// amount-due picture.
type CardHandler interface {
	CreateCard(w http.ResponseWriter, r *http.Request)
	ListCards(w http.ResponseWriter, r *http.Request)
	GetCard(w http.ResponseWriter, r *http.Request)
	UpdateCard(w http.ResponseWriter, r *http.Request)
	DeleteCard(w http.ResponseWriter, r *http.Request)
}

// ConfigHandler exposes runtime flags the frontend needs before it can
// decide how to render at all (today: whether to enforce the OIDC login
// guard). See config_handler.go's doc comment for why this is a minimal
// seed rather than BACK-02/BACK-09's final shape.
type ConfigHandler interface {
	GetConfig(w http.ResponseWriter, r *http.Request)
}

// CurrencyHandler exposes the user-extendable currency registry backing
// the frontend's currency dropdown.
type CurrencyHandler interface {
	ListCurrencies(w http.ResponseWriter, r *http.Request)
	AddCurrency(w http.ResponseWriter, r *http.Request)
}

// ImportHandler exposes BACK-03's CSV history backfill: the spec the
// frontend renders (not hardcoded) and the import endpoint itself. Its
// revert direction lives on ExportHandler instead (BACK-09's fuller
// export, with include_cancelled support) — see that type's doc comment.
type ImportHandler interface {
	GetImportSpec(w http.ResponseWriter, r *http.Request)
	ImportMovements(w http.ResponseWriter, r *http.Request)
}

// ExportHandler exposes BACK-09's CSV export: the user's own movement
// history in exactly BACK-03's import model (see internal/infrastructure/csv
// for the shared column model), available in every mode so data is always
// portable — not standalone-only.
type ExportHandler interface {
	ExportMovements(w http.ResponseWriter, r *http.Request)
}

// ExchangeRateHandler exposes user-managed, historical exchange rates
// against USD (BACK-11) — reference data the user maintains themselves,
// no external rate API involved.
type ExchangeRateHandler interface {
	SetExchangeRate(w http.ResponseWriter, r *http.Request)
	ListExchangeRates(w http.ResponseWriter, r *http.Request)
	DeleteExchangeRate(w http.ResponseWriter, r *http.Request)
}

// SettingsHandler exposes a user's own settings (BACK-13): entitlement
// (operator/billing-controlled, read-only here) and preference
// (user-controlled — ledger sync on/off today).
type SettingsHandler interface {
	GetSettings(w http.ResponseWriter, r *http.Request)
	PatchSettings(w http.ResponseWriter, r *http.Request)
}

// MovementHandler exposes financial-tracker's own API. It never talks to
// the database or ledger-service directly - it only calls application
// code, which depends on the application repository interfaces.
type MovementHandler interface {
	CreateMovement(w http.ResponseWriter, r *http.Request)
	GetMovement(w http.ResponseWriter, r *http.Request)
	ListMovements(w http.ResponseWriter, r *http.Request)
	UpdateMovement(w http.ResponseWriter, r *http.Request)
	CancelMovement(w http.ResponseWriter, r *http.Request)
	CancelCreditCardPurchase(w http.ResponseWriter, r *http.Request)
	Sync(w http.ResponseWriter, r *http.Request)
	ListCategories(w http.ResponseWriter, r *http.Request)
	Cashflow(w http.ResponseWriter, r *http.Request)
}

// RecurringRuleHandler exposes recurring movement rules (BACK-07) — rent,
// salary, subscriptions and the like, generated on schedule by
// application/recurring rather than re-entered every month.
type RecurringRuleHandler interface {
	CreateRecurringRule(w http.ResponseWriter, r *http.Request)
	ListRecurringRules(w http.ResponseWriter, r *http.Request)
	UpdateRecurringRule(w http.ResponseWriter, r *http.Request)
}

// ArchiveHandler exposes BACK-15's "no cloud" local archive tier: the
// per-user setting, the full-account export the frontend encrypts
// client-side, and the import that restores a (already-decrypted) one.
type ArchiveHandler interface {
	GetLocalArchiveSetting(w http.ResponseWriter, r *http.Request)
	SetLocalArchiveSetting(w http.ResponseWriter, r *http.Request)
	ExportArchive(w http.ResponseWriter, r *http.Request)
	ImportArchive(w http.ResponseWriter, r *http.Request)
}

// TransferHandler exposes account-to-account transfers: a linked
// debit/credit pair of movements that nets to zero, so it never changes
// the user's overall net worth.
type TransferHandler interface {
	CreateTransfer(w http.ResponseWriter, r *http.Request)
	CancelTransfer(w http.ResponseWriter, r *http.Request)
}

// UserHandler exposes the authenticated caller's own profile. There is no
// create/update endpoint — the row is provisioned and kept in sync by the
// auth middleware's EnsureUser call (BACK-02), not by a client request.
type UserHandler interface {
	Me(w http.ResponseWriter, r *http.Request)
}

// PaymentMethodHandler exposes the user-extendable payment-method
// registry (BACK-17), replacing the old fixed enum. ListPaymentMethods
// itself isn't here — it's exposed through MovementHandler.ListCategories
// (GET /categories), same as CategoryHandler leaves listing to that
// endpoint too.
type PaymentMethodHandler interface {
	CreatePaymentMethod(w http.ResponseWriter, r *http.Request)
	UpdatePaymentMethod(w http.ResponseWriter, r *http.Request)
	DeletePaymentMethod(w http.ResponseWriter, r *http.Request)
}
