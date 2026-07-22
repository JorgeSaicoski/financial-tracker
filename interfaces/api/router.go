package api

import (
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/interfaces/api/handlers"
)

func NewRouter(
	movementHandler handlers.MovementHandler,
	accountHandler handlers.AccountHandler,
	currencyHandler handlers.CurrencyHandler,
	transferHandler handlers.TransferHandler,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /movements", movementHandler.CreateMovement)
	mux.HandleFunc("GET /movements", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("id") {
			movementHandler.GetMovement(w, r)
		} else {
			movementHandler.ListMovements(w, r)
		}
	})
	mux.HandleFunc("PATCH /movements/{id}", movementHandler.UpdateMovement)
	mux.HandleFunc("POST /movements/{id}/cancel", movementHandler.CancelMovement)
	mux.HandleFunc("POST /credit-card-purchases/{id}/cancel", movementHandler.CancelCreditCardPurchase)
	mux.HandleFunc("POST /sync", movementHandler.Sync)
	mux.HandleFunc("GET /categories", movementHandler.ListCategories)
	mux.HandleFunc("GET /cashflow", movementHandler.Cashflow)

	mux.HandleFunc("GET /accounts", accountHandler.ListAccounts)
	mux.HandleFunc("POST /accounts", accountHandler.CreateAccount)
	mux.HandleFunc("POST /accounts/{id}/balance", accountHandler.ReportBalance)

	mux.HandleFunc("GET /currencies", currencyHandler.ListCurrencies)
	mux.HandleFunc("POST /currencies", currencyHandler.AddCurrency)

	mux.HandleFunc("POST /transfers", transferHandler.CreateTransfer)
	mux.HandleFunc("POST /transfers/{id}/cancel", transferHandler.CancelTransfer)

	return withCORS(mux)
}

// withCORS is a permissive, dev-only CORS layer so the Svelte app (running
// on its own port) can call this API directly. Tighten this before any
// non-local deployment.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
