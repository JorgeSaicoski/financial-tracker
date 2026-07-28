package main

import (
	"context"
	"database/sql"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	billingapp "github.com/JorgeSaicoski/financial-tracker/internal/application/billing"
	recurringapp "github.com/JorgeSaicoski/financial-tracker/internal/application/recurring"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	syncapp "github.com/JorgeSaicoski/financial-tracker/internal/application/sync"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/authentik"
	billinginfra "github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/billing"
	cryptox "github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/crypto"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/ledgerservice"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/postgresql"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/simpleauth"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/sqlite"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/handlers"
	applogger "github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
	"github.com/JorgeSaicoski/financial-tracker/internal/webui"
)

func main() {
	log := applogger.New()

	ledgerServiceURL := envOr("LEDGER_SERVICE_URL", "http://localhost:8080")

	// DEFAULT_USER_ID is now only a dev-mode escape hatch (see
	// authDisabled below) — real requests get user_id from their verified
	// Authentik token (BACK-02). Must be a lowercase UUID, since that's
	// what ledger-service's validator requires. Standalone (BACK-09)
	// always uses this same fixed id — there is only ever one user.
	defaultUserID := envOr("DEFAULT_USER_ID", "00000000-0000-0000-0000-000000000001")
	defaultCurrency := envOr("DEFAULT_CURRENCY", "usd")
	port := envOr("PORT", "8081")
	// "*" for local dev (Svelte dev server on its own port); INFRA-03's
	// proxy sets this to the proxied origin in deploy/compose.yaml once
	// frontend+API share one hostname.
	corsAllowedOrigin := envOr("CORS_ALLOWED_ORIGIN", "*")

	// standalone (BACK-09): one binary, no server, no account — accepts
	// either STANDALONE=true or a --standalone flag, since a
	// single-binary distribution is exactly the case where a user is
	// more likely to reach for a flag than an env var.
	standalone := boolEnvOr(log, "STANDALONE", false) || slices.Contains(os.Args[1:], "--standalone")

	dbDriver := envOr("DB_DRIVER", "sqlite")
	if standalone && dbDriver != "sqlite" {
		log.Info("STANDALONE=true forces DB_DRIVER=sqlite (was %q)", dbDriver)
	}
	if standalone {
		dbDriver = "sqlite"
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		if standalone {
			dbPath = standaloneDefaultDBPath(log)
		} else {
			dbPath = "./data/financial-tracker.db"
		}
	}

	// authDisabled=true skips Authentik token verification entirely and
	// attributes every request to defaultUserID — for local dev,
	// single-user self-hosting, and standalone (which has no account
	// system at all — see BACK-09) only. Defaults to false: a deployment
	// that forgets to set OIDC_ISSUER_URL fails loudly at startup rather
	// than silently running with no auth.
	authDisabled := standalone || boolEnvOr(log, "AUTH_DISABLED", false)
	// AUTH_PROVIDER (BACK-20) picks which services.IdentityVerifier
	// cmd/api constructs — "authentik" (default, unchanged behavior) or
	// "simple" (infrastructure/simpleauth: any other provider speaking
	// the same OIDC-like iss/sub/exp/aud + JWKS contract). Mirrors
	// DB_DRIVER's switch-on-a-string shape below. Irrelevant when
	// AUTH_DISABLED=true.
	authProvider := envOr("AUTH_PROVIDER", "authentik")
	oidcIssuerURL := os.Getenv("OIDC_ISSUER_URL")
	oidcJWKSURL := os.Getenv("OIDC_JWKS_URL") // optional override, skips discovery
	// Defaults to PUBLIC_OIDC_CLIENT_ID (deploy/.env.example's existing
	// var, already "financial-tracker" in the deployed Authentik
	// blueprint) since Authentik sets a token's aud to the requesting
	// client_id — so aud validation works out of the box in this stack
	// without a separate env var to keep in sync. Set OIDC_AUDIENCE
	// explicitly to override.
	oidcAudience := envOr("OIDC_AUDIENCE", envOr("PUBLIC_OIDC_CLIENT_ID", ""))
	// AUTH_PROVIDER=simple's own, independent config namespace — never
	// read unless authProvider is actually "simple", so it's safe to
	// leave these unset in every other deployment.
	simpleAuthIssuerURL := os.Getenv("SIMPLE_AUTH_ISSUER_URL")
	simpleAuthJWKSURL := os.Getenv("SIMPLE_AUTH_JWKS_URL")
	simpleAuthAudience := os.Getenv("SIMPLE_AUTH_AUDIENCE")

	// FRONT-04's GET /config: tells the frontend whether to enforce its
	// own login guard. Now that BACK-02's real server-side verification
	// exists, this is just the inverse of AUTH_DISABLED rather than a
	// separate flag — the frontend guard and the API's own enforcement
	// stay in lockstep with nothing to keep in sync (this folds together
	// the two flags the FRONT-04 PR description flagged as needing
	// reconciling once BACK-02 landed).
	authEnabled := !authDisabled

	syncInterval := durationEnvOr(log, "SYNC_INTERVAL", 30*time.Second)
	retryCooldown := durationEnvOr(log, "SYNC_RETRY_COOLDOWN", 60*time.Second)
	recurringInterval := durationEnvOr(log, "RECURRING_INTERVAL", 1*time.Hour)

	// BACK-19: paid cloud-storage subscription. Reference price is an
	// annual USD figure in cents (1000 = $10.00/year, the ticket's
	// anchor price — not final, see its "Open decisions"); grace period
	// is how long a past_due subscription keeps its entitlement before
	// the sweep lapses it ("a late card shouldn't cut off access
	// instantly").
	billingReferencePriceUSDCents := int64(intEnvOr(log, "BILLING_REFERENCE_PRICE_USD_CENTS", 1000))
	billingGracePeriodDays := intEnvOr(log, "BILLING_GRACE_PERIOD_DAYS", 7)
	billingSweepInterval := durationEnvOr(log, "BILLING_SWEEP_INTERVAL", time.Hour)

	// Infrastructure: the local database (SQLite by default, or Postgres
	// when DB_DRIVER=postgres) is the source of truth; ledger-service is
	// only reached by the background sync, so requests keep working while
	// it's down.
	var (
		db                  *sql.DB
		err                 error
		movementRepo        repositories.MovementRepository
		purchaseRepo        repositories.CreditCardPurchaseRepository
		accountRepo         repositories.AccountRepository
		cardRepo            repositories.CardRepository
		currencyRepo        repositories.CurrencyRepository
		categoryRepo        repositories.CategoryRepository
		exchangeRateRepo    repositories.ExchangeRateRepository
		recurringRuleRepo   repositories.RecurringRuleRepository
		localArchiveRepo    repositories.LocalArchiveSettingsRepository
		paymentMethodRepo   repositories.PaymentMethodRepository
		planRepo            repositories.PlanRepository
		userRepo            repositories.UserRepository
		settingsRepo        repositories.UserSettingsRepository
		ledgerPseudonymRepo repositories.LedgerPseudonymRepository
		subscriptionRepo    repositories.SubscriptionRepository
		limitsRepo          repositories.LimitsRepository
	)

	switch dbDriver {
	case "postgres":
		databaseURL := os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			log.Error("DATABASE_URL is required when DB_DRIVER=postgres")
			os.Exit(1)
		}
		poolConfig := postgresql.PoolConfig{
			MaxOpenConns:    intEnvOr(log, "POSTGRES_MAX_OPEN_CONNS", postgresql.DefaultPoolConfig.MaxOpenConns),
			MaxIdleConns:    intEnvOr(log, "POSTGRES_MAX_IDLE_CONNS", postgresql.DefaultPoolConfig.MaxIdleConns),
			ConnMaxLifetime: durationEnvOr(log, "POSTGRES_CONN_MAX_LIFETIME", postgresql.DefaultPoolConfig.ConnMaxLifetime),
			ConnMaxIdleTime: durationEnvOr(log, "POSTGRES_CONN_MAX_IDLE_TIME", postgresql.DefaultPoolConfig.ConnMaxIdleTime),
		}
		db, err = postgresql.Open(databaseURL, poolConfig)
		if err != nil {
			log.Error("opening database failed: %v", err)
			os.Exit(1)
		}
		if err := postgresql.Migrate(db); err != nil {
			log.Error("migrating database failed: %v", err)
			os.Exit(1)
		}

		// BACK-16: field-level envelope encryption for
		// movements.description/accounts.name — Postgres ("cloud
		// storage") only, since it's the only backend a stolen disk/DB
		// dump threat model applies to. Required at startup, not
		// optional, so a deployment can't silently run without it.
		masterKeyB64 := os.Getenv("ENCRYPTION_MASTER_KEY")
		if masterKeyB64 == "" {
			log.Error("ENCRYPTION_MASTER_KEY is required when DB_DRIVER=postgres (BACK-16: encrypts movements.description/accounts.name at rest). Generate with: openssl rand -base64 32")
			os.Exit(1)
		}
		masterKey, err := cryptox.ParseMasterKey(masterKeyB64)
		if err != nil {
			log.Error("ENCRYPTION_MASTER_KEY: %v", err)
			os.Exit(1)
		}
		fieldCryptor := cryptox.NewFieldCryptor(masterKey, postgresql.NewUserDataKeyRepository(db))

		movementRepo = cryptox.NewEncryptingMovementRepository(postgresql.NewMovementRepository(db), fieldCryptor)
		purchaseRepo = postgresql.NewCreditCardPurchaseRepository(db)
		accountRepo = cryptox.NewEncryptingAccountRepository(postgresql.NewAccountRepository(db), fieldCryptor)
		cardRepo = postgresql.NewCardRepository(db)
		currencyRepo = postgresql.NewCurrencyRepository(db)
		categoryRepo = postgresql.NewCategoryRepository(db)
		exchangeRateRepo = postgresql.NewExchangeRateRepository(db)
		recurringRuleRepo = postgresql.NewRecurringRuleRepository(db)
		localArchiveRepo = postgresql.NewLocalArchiveSettingsRepository(db)
		paymentMethodRepo = postgresql.NewPaymentMethodRepository(db)
		planRepo = postgresql.NewPlanRepository(db)
		userRepo = postgresql.NewUserRepository(db)
		settingsRepo = postgresql.NewUserSettingsRepository(db)
		ledgerPseudonymRepo = postgresql.NewLedgerPseudonymRepository(db)
		subscriptionRepo = postgresql.NewSubscriptionRepository(db)
		limitsRepo = postgresql.NewLimitsRepository(db)
	case "sqlite":
		db, err = sqlite.Open(dbPath)
		if err != nil {
			log.Error("opening database failed: %v", err)
			os.Exit(1)
		}
		if err := sqlite.Migrate(db); err != nil {
			log.Error("migrating database failed: %v", err)
			os.Exit(1)
		}
		movementRepo = sqlite.NewMovementRepository(db)
		purchaseRepo = sqlite.NewCreditCardPurchaseRepository(db)
		accountRepo = sqlite.NewAccountRepository(db)
		cardRepo = sqlite.NewCardRepository(db)
		currencyRepo = sqlite.NewCurrencyRepository(db)
		categoryRepo = sqlite.NewCategoryRepository(db)
		exchangeRateRepo = sqlite.NewExchangeRateRepository(db)
		recurringRuleRepo = sqlite.NewRecurringRuleRepository(db)
		localArchiveRepo = sqlite.NewLocalArchiveSettingsRepository(db)
		paymentMethodRepo = sqlite.NewPaymentMethodRepository(db)
		planRepo = sqlite.NewPlanRepository(db)
		userRepo = sqlite.NewUserRepository(db)
		settingsRepo = sqlite.NewUserSettingsRepository(db)
		ledgerPseudonymRepo = sqlite.NewLedgerPseudonymRepository(db)
		subscriptionRepo = sqlite.NewSubscriptionRepository(db)
		limitsRepo = sqlite.NewLimitsRepository(db)
	default:
		log.Error("unknown DB_DRIVER %q (want sqlite or postgres)", dbDriver)
		os.Exit(1)
	}
	defer db.Close()

	// BACK-16: pseudonymous ledger sync — required for both drivers,
	// since ledger sync itself is available regardless of DB_DRIVER.
	ledgerHMACKeyB64 := os.Getenv("LEDGER_HMAC_KEY")
	if ledgerHMACKeyB64 == "" {
		log.Error("LEDGER_HMAC_KEY is required (BACK-16: pseudonymizes ledger-service sync). Generate with: openssl rand -base64 32")
		os.Exit(1)
	}
	ledgerHMACKey, err := cryptox.ParseHMACKey(ledgerHMACKeyB64)
	if err != nil {
		log.Error("LEDGER_HMAC_KEY: %v", err)
		os.Exit(1)
	}
	ledgerPseudonymizer := cryptox.NewLedgerPseudonymizer(ledgerHMACKey, ledgerPseudonymRepo)

	// BACK-19: POST /billing/webhook authenticity — required at startup
	// like LEDGER_HMAC_KEY above, so a deployment can't silently accept
	// unsigned billing events.
	billingWebhookSecretB64 := os.Getenv("BILLING_WEBHOOK_SECRET")
	if billingWebhookSecretB64 == "" {
		log.Error("BILLING_WEBHOOK_SECRET is required (BACK-19: authenticates POST /billing/webhook). Generate with: openssl rand -base64 32")
		os.Exit(1)
	}
	billingWebhookSecret, err := cryptox.ParseHMACKey(billingWebhookSecretB64)
	if err != nil {
		log.Error("BILLING_WEBHOOK_SECRET: %v", err)
		os.Exit(1)
	}
	billingWebhookVerifier := billinginfra.NewHMACWebhookVerifier(billingWebhookSecret)

	ledgerClient := ledgerservice.NewClient(ledgerServiceURL)
	ledgerGateway := ledgerservice.NewLedgerGateway(ledgerClient, ledgerPseudonymizer)
	syncService := syncapp.NewService(movementRepo, settingsRepo, ledgerGateway, log, retryCooldown)
	recurringService := recurringapp.NewService(recurringRuleRepo, log)

	createMovement := usecases.NewCreateMovement(movementRepo, accountRepo, cardRepo, paymentMethodRepo, planRepo, categoryRepo, settingsRepo)
	createPurchase := usecases.NewCreateCreditCardPurchase(purchaseRepo, cardRepo, categoryRepo, settingsRepo)
	getMovement := usecases.NewGetMovement(movementRepo)
	listMovements := usecases.NewListMovements(movementRepo)
	updateMovement := usecases.NewUpdateMovement(movementRepo, accountRepo, paymentMethodRepo, planRepo, categoryRepo, syncService)
	cancelMovement := usecases.NewCancelMovement(movementRepo, syncService)
	cancelPurchase := usecases.NewCancelCreditCardPurchase(purchaseRepo, movementRepo, syncService)
	getCashflow := usecases.NewGetCashflow(movementRepo, accountRepo)
	createAccount := usecases.NewCreateAccount(accountRepo, currencyRepo)
	listAccounts := usecases.NewListAccounts(accountRepo, movementRepo)
	reportBalance := usecases.NewReportAccountBalance(accountRepo, movementRepo)
	listAccountSnapshots := usecases.NewListAccountSnapshots(accountRepo, movementRepo)
	createCard := usecases.NewCreateCard(cardRepo)
	listCards := usecases.NewListCards(cardRepo, movementRepo)
	getCard := usecases.NewGetCard(cardRepo, movementRepo)
	updateCard := usecases.NewUpdateCard(cardRepo)
	deleteCard := usecases.NewDeleteCard(cardRepo)
	listCurrencies := usecases.NewListCurrencies(currencyRepo)
	addCurrency := usecases.NewAddCurrency(currencyRepo)
	transferBetweenAccounts := usecases.NewTransferBetweenAccounts(movementRepo, accountRepo, planRepo, settingsRepo)
	cancelTransfer := usecases.NewCancelTransfer(movementRepo, syncService)
	setExchangeRate := usecases.NewSetExchangeRate(exchangeRateRepo, currencyRepo)
	listExchangeRates := usecases.NewListExchangeRates(exchangeRateRepo)
	deleteExchangeRate := usecases.NewDeleteExchangeRate(exchangeRateRepo)
	createRecurringRule := usecases.NewCreateRecurringRule(recurringRuleRepo, accountRepo, paymentMethodRepo, categoryRepo)
	listRecurringRules := usecases.NewListRecurringRules(recurringRuleRepo)
	updateRecurringRule := usecases.NewUpdateRecurringRule(recurringRuleRepo, accountRepo, paymentMethodRepo, categoryRepo)
	getLocalArchiveSetting := usecases.NewGetLocalArchiveSetting(localArchiveRepo)
	setLocalArchiveSetting := usecases.NewSetLocalArchiveSetting(localArchiveRepo)
	exportArchive := usecases.NewExportArchive(accountRepo, movementRepo, purchaseRepo)
	importArchive := usecases.NewImportArchive(accountRepo, movementRepo, purchaseRepo, categoryRepo)
	createPaymentMethod := usecases.NewCreatePaymentMethod(paymentMethodRepo)
	listPaymentMethods := usecases.NewListPaymentMethods(paymentMethodRepo)
	updatePaymentMethod := usecases.NewUpdatePaymentMethod(paymentMethodRepo)
	deletePaymentMethod := usecases.NewDeletePaymentMethod(paymentMethodRepo)
	createPlan := usecases.NewCreatePlan(planRepo, accountRepo)
	listPlans := usecases.NewListPlans(planRepo, movementRepo)
	getPlan := usecases.NewGetPlan(planRepo, movementRepo)
	updatePlan := usecases.NewUpdatePlan(planRepo)
	importMovements := usecases.NewImportMovements(movementRepo, accountRepo, currencyRepo, paymentMethodRepo, categoryRepo)
	exportMovements := usecases.NewExportMovements(movementRepo, accountRepo)
	ensureUser := usecases.NewEnsureUser(userRepo, settingsRepo)
	getUser := usecases.NewGetUser(userRepo)
	getSettings := usecases.NewGetUserSettings(settingsRepo, subscriptionRepo)
	updateSettings := usecases.NewUpdateUserSettings(settingsRepo, movementRepo, categoryRepo, subscriptionRepo)
	processBillingWebhook := usecases.NewProcessBillingWebhook(subscriptionRepo, settingsRepo)
	getBillingPlan := usecases.NewGetBillingPlan(exchangeRateRepo, currencyRepo, billingReferencePriceUSDCents)
	createCategory := usecases.NewCreateCategory(categoryRepo, limitsRepo)
	listCategories := usecases.NewListCategories(categoryRepo)
	updateCategory := usecases.NewUpdateCategory(categoryRepo)
	deleteCategory := usecases.NewDeleteCategory(categoryRepo, settingsRepo)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	if standalone {
		// Forces the fixed local user's effective ledger sync off (BACK-13's
		// existing mechanism) so every movement is created with
		// sync_status="local" instead of a "pending" row nothing will ever
		// pick up, and cancels always void (cancel_movement.go only
		// reverses a *synced* movement, which can now never happen) — see
		// the ticket's "Open decisions" note in plan.md for why this reuses
		// BACK-13 rather than inventing a second terminal state.
		syncDisabled := false
		if _, err := updateSettings.Execute(ctx, defaultUserID, usecases.UpdateUserSettingsInput{LedgerSyncEnabled: &syncDisabled}); err != nil {
			log.Error("standalone: disabling ledger sync for the local user failed: %v", err)
			os.Exit(1)
		}
	}

	movementHandler := handlers.NewMovementHandler(
		createMovement,
		createPurchase,
		getMovement,
		listMovements,
		updateMovement,
		cancelMovement,
		cancelPurchase,
		getCashflow,
		listPaymentMethods,
		listCategories,
		syncService,
		defaultCurrency,
		log,
	)
	accountHandler := handlers.NewAccountHandler(createAccount, listAccounts, reportBalance, listAccountSnapshots, log)
	cardHandler := handlers.NewCardHandler(createCard, listCards, getCard, updateCard, deleteCard, log)
	currencyHandler := handlers.NewCurrencyHandler(listCurrencies, addCurrency, log)
	categoryHandler := handlers.NewCategoryHandler(createCategory, updateCategory, deleteCategory, log)
	transferHandler := handlers.NewTransferHandler(transferBetweenAccounts, cancelTransfer, log)
	exchangeRateHandler := handlers.NewExchangeRateHandler(setExchangeRate, listExchangeRates, deleteExchangeRate, log)
	recurringRuleHandler := handlers.NewRecurringRuleHandler(createRecurringRule, listRecurringRules, updateRecurringRule, defaultCurrency, log)
	archiveHandler := handlers.NewArchiveHandler(getLocalArchiveSetting, setLocalArchiveSetting, exportArchive, importArchive, log)
	importHandler := handlers.NewImportHandler(importMovements, listAccounts, listCurrencies, listPaymentMethods, listCategories, log)
	exportHandler := handlers.NewExportHandler(exportMovements, log)
	paymentMethodHandler := handlers.NewPaymentMethodHandler(createPaymentMethod, updatePaymentMethod, deletePaymentMethod, log)
	planHandler := handlers.NewPlanHandler(createPlan, listPlans, getPlan, updatePlan, log)
	settingsHandler := handlers.NewSettingsHandler(getSettings, updateSettings, log)
	userHandler := handlers.NewUserHandler(getUser, log)
	configHandler := handlers.NewConfigHandler(standalone, authEnabled, log)
	billingHandler := handlers.NewBillingHandler(processBillingWebhook, getBillingPlan, billingWebhookVerifier, log)

	// Auth: AUTH_DISABLED is a dev-only escape hatch, off by default. A
	// deployment that leaves OIDC_ISSUER_URL unset without explicitly
	// opting into AUTH_DISABLED=true fails fast at startup rather than
	// silently serving every request as the same fixed user. Which
	// services.IdentityVerifier gets constructed below is a config
	// decision (AUTH_PROVIDER), not a code change (BACK-20) — the
	// application layer and interfaces/api know nothing about Authentik,
	// JWT, or JWKS; only this switch does.
	var authMiddleware api.AuthMiddleware
	if authDisabled {
		if standalone {
			log.Info("standalone: no auth — every request is attributed to the fixed local user %s", defaultUserID)
		} else {
			log.Info("AUTH_DISABLED=true: skipping Authentik token verification — every request is attributed to DEFAULT_USER_ID=%s. Do not use this in a real deployment.", defaultUserID)
		}
		authMiddleware = api.DevUserMiddleware(defaultUserID, ensureUser, log)
	} else {
		// A dedicated client with a timeout, not http.DefaultClient: a stalled
		// OIDC discovery/JWKS fetch must not be able to hang request auth
		// indefinitely.
		oidcHTTPClient := &http.Client{Timeout: 10 * time.Second}

		var verifier services.IdentityVerifier
		switch authProvider {
		case "authentik":
			if oidcIssuerURL == "" {
				log.Error("OIDC_ISSUER_URL is required when AUTH_PROVIDER=authentik and AUTH_DISABLED is not true")
				os.Exit(1)
			}
			verifier = authentik.NewVerifier(oidcIssuerURL, oidcAudience, oidcJWKSURL, oidcHTTPClient, log)
			log.Info("auth: validating Authorization bearer tokens against OIDC issuer %s (audience %q)", oidcIssuerURL, oidcAudience)
		case "simple":
			if simpleAuthIssuerURL == "" {
				log.Error("SIMPLE_AUTH_ISSUER_URL is required when AUTH_PROVIDER=simple")
				os.Exit(1)
			}
			verifier = simpleauth.NewVerifier(simpleAuthIssuerURL, simpleAuthAudience, simpleAuthJWKSURL, oidcHTTPClient, log)
			log.Info("auth: validating Authorization bearer tokens against issuer %s (audience %q) via AUTH_PROVIDER=simple", simpleAuthIssuerURL, simpleAuthAudience)
		default:
			log.Error("unknown AUTH_PROVIDER %q (want authentik or simple)", authProvider)
			os.Exit(1)
		}
		authMiddleware = api.Middleware(verifier, ensureUser, log)
	}

	// frontendFS is nil (not an error) when nobody ran the real frontend
	// build before compiling this binary — see internal/webui's doc
	// comment; the router serves a clear explanation instead of the
	// embedded SPA in that case. Only relevant in standalone: the
	// deployed stack always serves the frontend as its own container
	// (INFRA-03).
	var frontendFS fs.FS
	if standalone {
		if embedded, ok := webui.FS(); ok {
			frontendFS = embedded
		} else {
			log.Info("standalone: no frontend embedded in this binary (see internal/webui's doc comment) — the API itself still works")
		}
	}

	router := api.NewRouter(movementHandler, accountHandler, cardHandler, currencyHandler, categoryHandler, transferHandler, exchangeRateHandler, recurringRuleHandler, archiveHandler, importHandler, exportHandler, paymentMethodHandler, planHandler, settingsHandler, userHandler, configHandler, billingHandler, authMiddleware, corsAllowedOrigin, standalone, frontendFS)

	if !standalone {
		// Standalone has no ledger-service to sync to at all — starting
		// this loop would just retry-fail forever against
		// LEDGER_SERVICE_URL's default (localhost:8080), which the
		// acceptance criteria explicitly rules out ("no ledger-service
		// calls ever attempted").
		syncService.Start(ctx, syncInterval)
	}
	recurringService.Start(ctx, recurringInterval)
	billingapp.NewService(subscriptionRepo, settingsRepo, billingGracePeriodDays, log).Start(ctx, billingSweepInterval)

	dbDescription := dbPath
	if dbDriver == "postgres" {
		dbDescription = "postgres"
	}
	addr := ":" + port
	if standalone {
		absPath, err := filepath.Abs(dbPath)
		if err != nil {
			absPath = dbPath
		}
		log.Info("financial-tracker running in STANDALONE mode on %s: no server, no account, no sync — data is one file at %s", addr, absPath)
	} else {
		log.Info("financial-tracker API listening on %s (db driver %s at %s, syncing to ledger-service at %s every %s)", addr, dbDriver, dbDescription, ledgerServiceURL, syncInterval)
	}
	log.Info("endpoints: GET /config | GET|PATCH /settings | GET /import/movements/spec | POST /import/movements | GET /export/movements | POST /movements | GET /movements | PATCH /movements/{id} | POST /movements/{id}/cancel | POST /credit-card-purchases/{id}/cancel | POST /sync | GET /categories | POST /categories | PATCH /categories/{id} | DELETE /categories/{id} | GET /cashflow | GET|POST /accounts | POST /accounts/{id}/balance | GET|POST /cards | GET|PATCH|DELETE /cards/{id} | GET|POST /currencies | POST /transfers | POST /transfers/{id}/cancel | GET|POST /exchange-rates | DELETE /exchange-rates/{id} | GET|POST /recurring-rules | PATCH /recurring-rules/{id} | GET|PUT /settings/local-archive | GET /export/archive | POST /import/archive | POST /payment-methods | PATCH /payment-methods/{id} | DELETE /payment-methods/{id} | GET|POST /plans | GET|PATCH /plans/{id} | GET /me | POST /billing/webhook | GET /billing/plan")

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Error("server failed: %v", err)
		os.Exit(1)
	}
}

// standaloneDefaultDBPath is DB_PATH's default when STANDALONE=true: an
// OS-appropriate per-user data directory instead of "./data" (which
// assumes a server's working directory, not a program a user
// double-clicks from anywhere). Falls back to "./data/financial-tracker.db"
// with a loud log if the OS doesn't define one — better than failing to
// start entirely.
func standaloneDefaultDBPath(log applogger.Logger) string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Error("standalone: os.UserConfigDir() failed (%v), falling back to ./data", err)
		return "./data/financial-tracker.db"
	}
	return filepath.Join(configDir, "financial-tracker", "financial-tracker.db")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func durationEnvOr(log applogger.Logger, key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Error("invalid %s %q, using default %s", key, raw, fallback)
		return fallback
	}
	return d
}

func intEnvOr(log applogger.Logger, key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		log.Error("invalid %s %q, using default %d", key, raw, fallback)
		return fallback
	}
	return n
}

// boolEnvOr parses a strict true/false (anything strconv.ParseBool
// accepts); unset or invalid falls back — used only for AUTH_DISABLED,
// where silently misreading a typo as "true" would be a real security
// hole, so an invalid value logs loudly rather than guessing.
func boolEnvOr(log applogger.Logger, key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		log.Error("invalid %s %q (want true or false), using default %v", key, raw, fallback)
		return fallback
	}
	return b
}
