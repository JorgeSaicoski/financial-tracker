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

// CurrencyHandler exposes the user-extendable currency registry backing
// the frontend's currency dropdown.
type CurrencyHandler interface {
	ListCurrencies(w http.ResponseWriter, r *http.Request)
	AddCurrency(w http.ResponseWriter, r *http.Request)
}

// ExchangeRateHandler exposes user-managed, historical exchange rates
// against USD (BACK-11) — reference data the user maintains themselves,
// no external rate API involved.
type ExchangeRateHandler interface {
	SetExchangeRate(w http.ResponseWriter, r *http.Request)
	ListExchangeRates(w http.ResponseWriter, r *http.Request)
	DeleteExchangeRate(w http.ResponseWriter, r *http.Request)
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
