package api

import (
	"io/fs"
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
	cardHandler handlers.CardHandler,
	currencyHandler handlers.CurrencyHandler,
	categoryHandler handlers.CategoryHandler,
	transferHandler handlers.TransferHandler,
	exchangeRateHandler handlers.ExchangeRateHandler,
	recurringRuleHandler handlers.RecurringRuleHandler,
	archiveHandler handlers.ArchiveHandler,
	importHandler handlers.ImportHandler,
	exportHandler handlers.ExportHandler,
	paymentMethodHandler handlers.PaymentMethodHandler,
	planHandler handlers.PlanHandler,
	settingsHandler handlers.SettingsHandler,
	userHandler handlers.UserHandler,
	configHandler handlers.ConfigHandler,
	billingHandler handlers.BillingHandler,
	authMiddleware AuthMiddleware,
	allowedOrigin string,
	// standalone (BACK-09) rejects /sync (there is no ledger-service to
	// push to) and, when frontendFS is non-nil, serves the embedded
	// SvelteKit build as a fallback for any path that isn't a registered
	// API route.
	standalone bool,
	frontendFS fs.FS,
) http.Handler {
	// Every route except /config needs a resolved user_id, so it lives on
	// its own mux wrapped in authMiddleware — see below for why /config
	// itself must stay outside that wrapping.
	protected := http.NewServeMux()

	protected.HandleFunc("GET /import/movements/spec", importHandler.GetImportSpec)
	protected.HandleFunc("POST /import/movements", importHandler.ImportMovements)
	protected.HandleFunc("GET /export/movements", exportHandler.ExportMovements)

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
	if standalone {
		// Explicit rather than just leaving the route unregistered: an
		// unregistered path here would still resolve to the "/" SPA
		// fallback registered below for the same (GET-agnostic) pattern
		// space, silently returning the app shell instead of an error —
		// a clear rejection is what BACK-09's acceptance criteria asks
		// for, not a bare 404 that happens to fall out of routing order.
		protected.HandleFunc("POST /sync", standaloneSyncRejectedHandler)
	} else {
		protected.HandleFunc("POST /sync", movementHandler.Sync)
	}
	protected.HandleFunc("GET /categories", movementHandler.ListCategories)
	protected.HandleFunc("POST /categories", categoryHandler.CreateCategory)
	protected.HandleFunc("PATCH /categories/{id}", categoryHandler.UpdateCategory)
	protected.HandleFunc("DELETE /categories/{id}", categoryHandler.DeleteCategory)
	protected.HandleFunc("GET /cashflow", movementHandler.Cashflow)

	protected.HandleFunc("GET /accounts", accountHandler.ListAccounts)
	protected.HandleFunc("POST /accounts", accountHandler.CreateAccount)
	protected.HandleFunc("POST /accounts/{id}/balance", accountHandler.ReportBalance)
	protected.HandleFunc("GET /accounts/{id}/balance", accountHandler.ListSnapshots)

	protected.HandleFunc("GET /cards", cardHandler.ListCards)
	protected.HandleFunc("POST /cards", cardHandler.CreateCard)
	protected.HandleFunc("GET /cards/{id}", cardHandler.GetCard)
	protected.HandleFunc("PATCH /cards/{id}", cardHandler.UpdateCard)
	protected.HandleFunc("DELETE /cards/{id}", cardHandler.DeleteCard)

	protected.HandleFunc("GET /currencies", currencyHandler.ListCurrencies)
	protected.HandleFunc("POST /currencies", currencyHandler.AddCurrency)

	protected.HandleFunc("POST /transfers", transferHandler.CreateTransfer)
	protected.HandleFunc("POST /transfers/{id}/cancel", transferHandler.CancelTransfer)

	protected.HandleFunc("GET /exchange-rates", exchangeRateHandler.ListExchangeRates)
	protected.HandleFunc("POST /exchange-rates", exchangeRateHandler.SetExchangeRate)
	protected.HandleFunc("DELETE /exchange-rates/{id}", exchangeRateHandler.DeleteExchangeRate)

	protected.HandleFunc("GET /recurring-rules", recurringRuleHandler.ListRecurringRules)
	protected.HandleFunc("POST /recurring-rules", recurringRuleHandler.CreateRecurringRule)
	protected.HandleFunc("PATCH /recurring-rules/{id}", recurringRuleHandler.UpdateRecurringRule)

	protected.HandleFunc("GET /settings/local-archive", archiveHandler.GetLocalArchiveSetting)
	protected.HandleFunc("PUT /settings/local-archive", archiveHandler.SetLocalArchiveSetting)
	protected.HandleFunc("GET /export/archive", archiveHandler.ExportArchive)
	protected.HandleFunc("POST /import/archive", archiveHandler.ImportArchive)

	protected.HandleFunc("POST /payment-methods", paymentMethodHandler.CreatePaymentMethod)
	protected.HandleFunc("PATCH /payment-methods/{id}", paymentMethodHandler.UpdatePaymentMethod)
	protected.HandleFunc("DELETE /payment-methods/{id}", paymentMethodHandler.DeletePaymentMethod)

	protected.HandleFunc("POST /plans", planHandler.CreatePlan)
	protected.HandleFunc("GET /plans", planHandler.ListPlans)
	protected.HandleFunc("GET /plans/{id}", planHandler.GetPlan)
	protected.HandleFunc("PATCH /plans/{id}", planHandler.UpdatePlan)

	protected.HandleFunc("GET /me", userHandler.Me)

	protected.HandleFunc("GET /billing/plan", billingHandler.GetPlan)

	if standalone {
		// Registered on protected (behind the no-op standalone auth
		// middleware, not unauthenticated) so it sits below every
		// explicit API pattern above — Go's ServeMux always prefers a
		// more specific match, so this only ever catches paths that
		// aren't one of this API's own routes (every real route above,
		// including /sync, is registered explicitly regardless of mode).
		if frontendFS != nil {
			protected.Handle("/", newSPAHandler(frontendFS))
		} else {
			protected.Handle("/", noFrontendEmbeddedHandler())
		}
	}

	mux := http.NewServeMux()
	// Unauthenticated by design (see config_handler.go): the frontend
	// calls this before it has a token to decide whether it needs one, so
	// it must not go through authMiddleware like everything else does —
	// mounted on the outer mux instead of protected.
	mux.HandleFunc("GET /config", configHandler.GetConfig)
	// Unauthenticated by user token like /config, but for a different
	// reason (BACK-19): the payment provider calling this has no
	// financial-tracker session to present a bearer token for.
	// billingHandler.Webhook checks the request's own signature header
	// instead — see its doc comment.
	mux.HandleFunc("POST /billing/webhook", billingHandler.Webhook)
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
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
