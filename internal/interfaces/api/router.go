package api

import (
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/handlers"
)

func NewRouter(
	movementHandler handlers.MovementHandler,
	accountHandler handlers.AccountHandler,
	currencyHandler handlers.CurrencyHandler,
	transferHandler handlers.TransferHandler,
	allowedOrigin string,
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

	return withCORS(mux, allowedOrigin)
}

// withCORS allows one configured origin (see cmd/api's CORS_ALLOWED_ORIGIN,
// defaulted to "*" for local dev where the Svelte dev server runs on its
// own port). Once INFRA-03's reverse proxy puts the frontend and API
// behind the same origin, deploy/compose.yaml locks this to that origin.
func withCORS(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
