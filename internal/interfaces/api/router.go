package api

import (
	"net/http"

	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/handlers"
)

// AuthMiddleware wraps a handler with authentication — either a real
// Authenticator.Middleware validating Authentik-issued JWTs, or
// DevUserMiddleware's AUTH_DISABLED escape hatch. cmd/api/main.go decides
// which and passes it in, so this package's routing logic doesn't need to
// know about env vars or JWKS at all.
type AuthMiddleware func(http.Handler) http.Handler

func NewRouter(
	movementHandler handlers.MovementHandler,
	accountHandler handlers.AccountHandler,
	currencyHandler handlers.CurrencyHandler,
	transferHandler handlers.TransferHandler,
	exchangeRateHandler handlers.ExchangeRateHandler,
	userHandler handlers.UserHandler,
	authMiddleware AuthMiddleware,
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

	mux.HandleFunc("GET /exchange-rates", exchangeRateHandler.ListExchangeRates)
	mux.HandleFunc("POST /exchange-rates", exchangeRateHandler.SetExchangeRate)
	mux.HandleFunc("DELETE /exchange-rates/{id}", exchangeRateHandler.DeleteExchangeRate)

	mux.HandleFunc("GET /me", userHandler.Me)

	// Auth wraps every route: user_id always comes from the verified
	// token (or the AUTH_DISABLED dev stand-in), never from a request
	// body or query string — see BACK-02. CORS wraps auth, not the other
	// way around, so a browser's OPTIONS preflight (which never carries
	// an Authorization header) gets its 204 from withCORS before auth
	// ever runs.
	return withCORS(authMiddleware(mux), allowedOrigin)
}

// withCORS allows one configured origin (see cmd/api's CORS_ALLOWED_ORIGIN;
// dev/docker-compose.yml defaults it to "*" for the Svelte dev server
// running on its own port — deploy/compose.yaml locks it to the real
// proxied origin per BACK-02). Authorization must be an allowed header so
// the browser will actually send the bearer token cross-origin; PATCH and
// DELETE are needed by BACK-04's update-movement and the exchange-rate
// delete endpoint.
func withCORS(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
