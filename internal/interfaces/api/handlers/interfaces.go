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
// frontend renders (not hardcoded), the import endpoint itself, and its
// revert direction (ExportMovements) — same fixed CSV model both ways,
// see internal/infrastructure/csv.
type ImportHandler interface {
	GetImportSpec(w http.ResponseWriter, r *http.Request)
	ImportMovements(w http.ResponseWriter, r *http.Request)
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
