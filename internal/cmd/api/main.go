package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strconv"
	"time"

	recurringapp "github.com/JorgeSaicoski/financial-tracker/internal/application/recurring"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	syncapp "github.com/JorgeSaicoski/financial-tracker/internal/application/sync"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/authentik"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/ledgerservice"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/postgresql"
	"github.com/JorgeSaicoski/financial-tracker/internal/infrastructure/sqlite"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api"
	"github.com/JorgeSaicoski/financial-tracker/internal/interfaces/api/handlers"
	applogger "github.com/JorgeSaicoski/financial-tracker/internal/pkg/logger"
)

func main() {
	ledgerServiceURL := envOr("LEDGER_SERVICE_URL", "http://localhost:8080")

	// DEFAULT_USER_ID is now only a dev-mode escape hatch (see
	// authDisabled below) — real requests get user_id from their verified
	// Authentik token (BACK-02). Must be a lowercase UUID, since that's
	// what ledger-service's validator requires.
	defaultUserID := envOr("DEFAULT_USER_ID", "00000000-0000-0000-0000-000000000001")
	defaultCurrency := envOr("DEFAULT_CURRENCY", "usd")
	port := envOr("PORT", "8081")
	// "*" for local dev (Svelte dev server on its own port); INFRA-03's
	// proxy sets this to the proxied origin in deploy/compose.yaml once
	// frontend+API share one hostname.
	corsAllowedOrigin := envOr("CORS_ALLOWED_ORIGIN", "*")
	dbPath := envOr("DB_PATH", "./data/financial-tracker.db")
	dbDriver := envOr("DB_DRIVER", "sqlite")

	log := applogger.New()

	// authDisabled=true skips Authentik token verification entirely and
	// attributes every request to defaultUserID — for local dev and
	// single-user self-hosting only. Defaults to false: a deployment that
	// forgets to set OIDC_ISSUER_URL fails loudly at startup instead of
	// silently running with no auth.
	authDisabled := boolEnvOr(log, "AUTH_DISABLED", false)
	oidcIssuerURL := os.Getenv("OIDC_ISSUER_URL")
	oidcJWKSURL := os.Getenv("OIDC_JWKS_URL") // optional override, skips discovery
	// Defaults to PUBLIC_OIDC_CLIENT_ID (deploy/.env.example's existing
	// var, already "financial-tracker" in the deployed Authentik
	// blueprint) since Authentik sets a token's aud to the requesting
	// client_id — so aud validation works out of the box in this stack
	// without a separate env var to keep in sync. Set OIDC_AUDIENCE
	// explicitly to override.
	oidcAudience := envOr("OIDC_AUDIENCE", envOr("PUBLIC_OIDC_CLIENT_ID", ""))

	// FRONT-04's GET /config: tells the frontend whether to enforce its
	// own login guard. Now that BACK-02's real server-side verification
	// exists, this is just the inverse of AUTH_DISABLED rather than a
	// separate flag — the frontend guard and the API's own enforcement
	// stay in lockstep with nothing to keep in sync (this folds together
	// the two flags the FRONT-04 PR description flagged as needing
	// reconciling once BACK-02 landed). `standalone` is hardcoded false
	// until BACK-09 exists.
	authEnabled := !authDisabled
	const standalone = false

	syncInterval := durationEnvOr(log, "SYNC_INTERVAL", 30*time.Second)
	retryCooldown := durationEnvOr(log, "SYNC_RETRY_COOLDOWN", 60*time.Second)
	recurringInterval := durationEnvOr(log, "RECURRING_INTERVAL", 1*time.Hour)

	// Infrastructure: the local database (SQLite by default, or Postgres
	// when DB_DRIVER=postgres) is the source of truth; ledger-service is
	// only reached by the background sync, so requests keep working while
	// it's down.
	var (
		db                *sql.DB
		err               error
		movementRepo      repositories.MovementRepository
		purchaseRepo      repositories.CreditCardPurchaseRepository
		accountRepo       repositories.AccountRepository
		currencyRepo      repositories.CurrencyRepository
		categoryRepo      repositories.CategoryRepository
		exchangeRateRepo  repositories.ExchangeRateRepository
		recurringRuleRepo repositories.RecurringRuleRepository
		localArchiveRepo  repositories.LocalArchiveSettingsRepository
		userRepo          repositories.UserRepository
		settingsRepo      repositories.UserSettingsRepository
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
		movementRepo = postgresql.NewMovementRepository(db)
		purchaseRepo = postgresql.NewCreditCardPurchaseRepository(db)
		accountRepo = postgresql.NewAccountRepository(db)
		currencyRepo = postgresql.NewCurrencyRepository(db)
		categoryRepo = postgresql.NewCategoryRepository(db)
		exchangeRateRepo = postgresql.NewExchangeRateRepository(db)
		recurringRuleRepo = postgresql.NewRecurringRuleRepository(db)
		localArchiveRepo = postgresql.NewLocalArchiveSettingsRepository(db)
		userRepo = postgresql.NewUserRepository(db)
		settingsRepo = postgresql.NewUserSettingsRepository(db)
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
		currencyRepo = sqlite.NewCurrencyRepository(db)
		categoryRepo = sqlite.NewCategoryRepository(db)
		exchangeRateRepo = sqlite.NewExchangeRateRepository(db)
		recurringRuleRepo = sqlite.NewRecurringRuleRepository(db)
		localArchiveRepo = sqlite.NewLocalArchiveSettingsRepository(db)
		userRepo = sqlite.NewUserRepository(db)
		settingsRepo = sqlite.NewUserSettingsRepository(db)
	default:
		log.Error("unknown DB_DRIVER %q (want sqlite or postgres)", dbDriver)
		os.Exit(1)
	}
	defer db.Close()

	ledgerClient := ledgerservice.NewClient(ledgerServiceURL)
	ledgerGateway := ledgerservice.NewLedgerGateway(ledgerClient)
	syncService := syncapp.NewService(movementRepo, settingsRepo, ledgerGateway, log, retryCooldown)
	recurringService := recurringapp.NewService(recurringRuleRepo, log)

	createMovement := usecases.NewCreateMovement(movementRepo, accountRepo, categoryRepo, settingsRepo)
	createPurchase := usecases.NewCreateCreditCardPurchase(purchaseRepo, categoryRepo, settingsRepo)
	getMovement := usecases.NewGetMovement(movementRepo)
	listMovements := usecases.NewListMovements(movementRepo)
	updateMovement := usecases.NewUpdateMovement(movementRepo, accountRepo, categoryRepo, syncService)
	cancelMovement := usecases.NewCancelMovement(movementRepo, syncService)
	cancelPurchase := usecases.NewCancelCreditCardPurchase(purchaseRepo, movementRepo, syncService)
	getCashflow := usecases.NewGetCashflow(movementRepo, accountRepo)
	createAccount := usecases.NewCreateAccount(accountRepo, currencyRepo)
	listAccounts := usecases.NewListAccounts(accountRepo, movementRepo)
	reportBalance := usecases.NewReportAccountBalance(accountRepo, movementRepo)
	listCurrencies := usecases.NewListCurrencies(currencyRepo)
	addCurrency := usecases.NewAddCurrency(currencyRepo)
	transferBetweenAccounts := usecases.NewTransferBetweenAccounts(movementRepo, accountRepo, settingsRepo, categoryRepo)
	cancelTransfer := usecases.NewCancelTransfer(movementRepo, syncService)
	setExchangeRate := usecases.NewSetExchangeRate(exchangeRateRepo, currencyRepo)
	listExchangeRates := usecases.NewListExchangeRates(exchangeRateRepo)
	deleteExchangeRate := usecases.NewDeleteExchangeRate(exchangeRateRepo)
	createRecurringRule := usecases.NewCreateRecurringRule(recurringRuleRepo, accountRepo, categoryRepo)
	listRecurringRules := usecases.NewListRecurringRules(recurringRuleRepo)
	updateRecurringRule := usecases.NewUpdateRecurringRule(recurringRuleRepo, accountRepo, categoryRepo)
	getLocalArchiveSetting := usecases.NewGetLocalArchiveSetting(localArchiveRepo)
	setLocalArchiveSetting := usecases.NewSetLocalArchiveSetting(localArchiveRepo)
	exportArchive := usecases.NewExportArchive(accountRepo, movementRepo, purchaseRepo)
	importArchive := usecases.NewImportArchive(accountRepo, movementRepo, purchaseRepo, categoryRepo)
	ensureUser := usecases.NewEnsureUser(userRepo)
	getUser := usecases.NewGetUser(userRepo)
	getSettings := usecases.NewGetUserSettings(settingsRepo)
	updateSettings := usecases.NewUpdateUserSettings(settingsRepo, movementRepo)
	createCategory := usecases.NewCreateCategory(categoryRepo)
	listCategories := usecases.NewListCategories(categoryRepo)
	updateCategory := usecases.NewUpdateCategory(categoryRepo)
	deleteCategory := usecases.NewDeleteCategory(categoryRepo)

	movementHandler := handlers.NewMovementHandler(
		createMovement,
		createPurchase,
		getMovement,
		listMovements,
		updateMovement,
		cancelMovement,
		cancelPurchase,
		getCashflow,
		listCategories,
		syncService,
		defaultCurrency,
		log,
	)
	accountHandler := handlers.NewAccountHandler(createAccount, listAccounts, reportBalance, log)
	currencyHandler := handlers.NewCurrencyHandler(listCurrencies, addCurrency, log)
	categoryHandler := handlers.NewCategoryHandler(createCategory, updateCategory, deleteCategory, log)
	transferHandler := handlers.NewTransferHandler(transferBetweenAccounts, cancelTransfer, log)
	exchangeRateHandler := handlers.NewExchangeRateHandler(setExchangeRate, listExchangeRates, deleteExchangeRate, log)
	recurringRuleHandler := handlers.NewRecurringRuleHandler(createRecurringRule, listRecurringRules, updateRecurringRule, defaultCurrency, log)
	archiveHandler := handlers.NewArchiveHandler(getLocalArchiveSetting, setLocalArchiveSetting, exportArchive, importArchive, log)
	settingsHandler := handlers.NewSettingsHandler(getSettings, updateSettings, log)
	userHandler := handlers.NewUserHandler(getUser, log)
	configHandler := handlers.NewConfigHandler(standalone, authEnabled, log)

	// Auth: AUTH_DISABLED is a dev-only escape hatch, off by default. A
	// deployment that leaves OIDC_ISSUER_URL unset without explicitly
	// opting into AUTH_DISABLED=true fails fast at startup rather than
	// silently serving every request as the same fixed user.
	var authMiddleware api.AuthMiddleware
	if authDisabled {
		log.Info("AUTH_DISABLED=true: skipping Authentik token verification — every request is attributed to DEFAULT_USER_ID=%s. Do not use this in a real deployment.", defaultUserID)
		authMiddleware = api.DevUserMiddleware(defaultUserID, ensureUser, log)
	} else {
		if oidcIssuerURL == "" {
			log.Error("OIDC_ISSUER_URL is required unless AUTH_DISABLED=true")
			os.Exit(1)
		}
		// A dedicated client with a timeout, not http.DefaultClient: a stalled
		// OIDC discovery/JWKS fetch must not be able to hang request auth
		// indefinitely.
		oidcHTTPClient := &http.Client{Timeout: 10 * time.Second}
		verifier := authentik.NewVerifier(oidcIssuerURL, oidcAudience, oidcJWKSURL, oidcHTTPClient, log)
		authMiddleware = api.Middleware(verifier, ensureUser, log)
		log.Info("auth: validating Authorization bearer tokens against OIDC issuer %s (audience %q)", oidcIssuerURL, oidcAudience)
	}

	router := api.NewRouter(movementHandler, accountHandler, currencyHandler, categoryHandler, transferHandler, exchangeRateHandler, recurringRuleHandler, archiveHandler, settingsHandler, userHandler, configHandler, authMiddleware, corsAllowedOrigin)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	syncService.Start(ctx, syncInterval)
	recurringService.Start(ctx, recurringInterval)

	dbDescription := dbPath
	if dbDriver == "postgres" {
		dbDescription = "postgres"
	}
	addr := ":" + port
	log.Info("financial-tracker API listening on %s (db driver %s at %s, syncing to ledger-service at %s every %s)", addr, dbDriver, dbDescription, ledgerServiceURL, syncInterval)
	log.Info("endpoints: GET /config | GET|PATCH /settings | POST /movements | GET /movements | PATCH /movements/{id} | POST /movements/{id}/cancel | POST /credit-card-purchases/{id}/cancel | POST /sync | GET /categories | POST /categories | PATCH /categories/{id} | DELETE /categories/{id} | GET /cashflow | GET|POST /accounts | POST /accounts/{id}/balance | GET|POST /currencies | POST /transfers | POST /transfers/{id}/cancel | GET|POST /exchange-rates | DELETE /exchange-rates/{id} | GET|POST /recurring-rules | PATCH /recurring-rules/{id} | GET|PUT /settings/local-archive | GET /export/archive | POST /import/archive | GET /me")

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Error("server failed: %v", err)
		os.Exit(1)
	}
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
