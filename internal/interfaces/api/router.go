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
	importHandler handlers.ImportHandler,
	settingsHandler handlers.SettingsHandler,
	userHandler handlers.UserHandler,
	configHandler handlers.ConfigHandler,
	authMiddleware AuthMiddleware,
	allowedOrigin string,
) http.Handler {
	// Every route except /config needs a resolved user_id, so it lives on
	// its own mux wrapped in authMiddleware — see below for why /config
	// itself must stay outside that wrapping.
	protected := http.NewServeMux()

	protected.HandleFunc("GET /import/movements/spec", importHandler.GetImportSpec)
	protected.HandleFunc("POST /import/movements", importHandler.ImportMovements)
	protected.HandleFunc("GET /export/movements", importHandler.ExportMovements)

	protected.HandleFunc("GET /settings", settingsHandler.GetSettings)
	protected.HandleFunc("PATCH /settings", settingsHandler.PatchSettings)

	protected.HandleFunc("POST /movements", movementHandler.CreateMovement)
	protected.HandleFunc("GET /movements", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("id") {
			movementHandler.GetMovement(w, r)
		} else {
			movementHandler.ListMovements(w, r)
		}
	})
	protected.HandleFunc("PATCH /movements/{id}", movementHandler.UpdateMovement)
	protected.HandleFunc("POST /movements/{id}/cancel", movementHandler.CancelMovement)
	protected.HandleFunc("POST /credit-card-purchases/{id}/cancel", movementHandler.CancelCreditCardPurchase)
	protected.HandleFunc("POST /sync", movementHandler.Sync)
	protected.HandleFunc("GET /categories", movementHandler.ListCategories)
	protected.HandleFunc("GET /cashflow", movementHandler.Cashflow)

	protected.HandleFunc("GET /accounts", accountHandler.ListAccounts)
	protected.HandleFunc("POST /accounts", accountHandler.CreateAccount)
	protected.HandleFunc("POST /accounts/{id}/balance", accountHandler.ReportBalance)

	protected.HandleFunc("GET /currencies", currencyHandler.ListCurrencies)
	protected.HandleFunc("POST /currencies", currencyHandler.AddCurrency)

	protected.HandleFunc("POST /transfers", transferHandler.CreateTransfer)
	protected.HandleFunc("POST /transfers/{id}/cancel", transferHandler.CancelTransfer)

	protected.HandleFunc("GET /exchange-rates", exchangeRateHandler.ListExchangeRates)
	protected.HandleFunc("POST /exchange-rates", exchangeRateHandler.SetExchangeRate)
	protected.HandleFunc("DELETE /exchange-rates/{id}", exchangeRateHandler.DeleteExchangeRate)

	protected.HandleFunc("GET /me", userHandler.Me)

	mux := http.NewServeMux()
	// Unauthenticated by design (see config_handler.go): the frontend
	// calls this before it has a token to decide whether it needs one, so
	// it must not go through authMiddleware like everything else does —
	// mounted on the outer mux instead of protected.
	mux.HandleFunc("GET /config", configHandler.GetConfig)
	// user_id always comes from the verified token (or the AUTH_DISABLED
	// dev stand-in), never from a request body or query string — see
	// BACK-02.
	mux.Handle("/", authMiddleware(protected))

	// CORS wraps everything, not the other way around, so a browser's
	// OPTIONS preflight (which never carries an Authorization header)
	// gets its 204 from withCORS before auth ever runs.
	return withCORS(mux, allowedOrigin)
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
		// Vary: Origin so a cache in front of this API (or the browser's own
		// HTTP cache) doesn't serve one origin's CORS-allowed response to a
		// different origin — matters even with allowedOrigin="*" today,
		// since a deployment can tighten it later without a corresponding
		// cache-config change.
		w.Header().Add("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
