package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/application/repositories"
	syncapp "github.com/JorgeSaicoski/financial-tracker/application/sync"
	"github.com/JorgeSaicoski/financial-tracker/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/infrastructure/ledgerservice"
	"github.com/JorgeSaicoski/financial-tracker/infrastructure/postgresql"
	"github.com/JorgeSaicoski/financial-tracker/infrastructure/sqlite"
	"github.com/JorgeSaicoski/financial-tracker/interfaces/api"
	"github.com/JorgeSaicoski/financial-tracker/interfaces/api/handlers"
	applogger "github.com/JorgeSaicoski/financial-tracker/pkg/logger"
)

func main() {
	ledgerServiceURL := envOr("LEDGER_SERVICE_URL", "http://localhost:8080")

	// MVP has no auth yet: every request without an explicit user_id is
	// attributed to this fixed dev user. Must be a lowercase UUID, since
	// that's what ledger-service's validator requires.
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

	syncInterval := durationEnvOr(log, "SYNC_INTERVAL", 30*time.Second)
	retryCooldown := durationEnvOr(log, "SYNC_RETRY_COOLDOWN", 60*time.Second)

	// Infrastructure: the local database (SQLite by default, or Postgres
	// when DB_DRIVER=postgres) is the source of truth; ledger-service is
	// only reached by the background sync, so requests keep working while
	// it's down.
	var (
		db               *sql.DB
		err              error
		movementRepo     repositories.MovementRepository
		purchaseRepo     repositories.CreditCardPurchaseRepository
		accountRepo      repositories.AccountRepository
		currencyRepo     repositories.CurrencyRepository
		exchangeRateRepo repositories.ExchangeRateRepository
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
		exchangeRateRepo = postgresql.NewExchangeRateRepository(db)
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
		exchangeRateRepo = sqlite.NewExchangeRateRepository(db)
	default:
		log.Error("unknown DB_DRIVER %q (want sqlite or postgres)", dbDriver)
		os.Exit(1)
	}
	defer db.Close()

	ledgerClient := ledgerservice.NewClient(ledgerServiceURL)
	ledgerGateway := ledgerservice.NewLedgerGateway(ledgerClient)
	syncService := syncapp.NewService(movementRepo, ledgerGateway, log, retryCooldown)

	createMovement := usecases.NewCreateMovement(movementRepo, accountRepo)
	createPurchase := usecases.NewCreateCreditCardPurchase(purchaseRepo)
	getMovement := usecases.NewGetMovement(movementRepo)
	listMovements := usecases.NewListMovements(movementRepo)
	updateMovement := usecases.NewUpdateMovement(movementRepo, accountRepo, syncService)
	cancelMovement := usecases.NewCancelMovement(movementRepo, syncService)
	cancelPurchase := usecases.NewCancelCreditCardPurchase(purchaseRepo, movementRepo, syncService)
	getCashflow := usecases.NewGetCashflow(movementRepo, accountRepo)
	createAccount := usecases.NewCreateAccount(accountRepo, currencyRepo)
	listAccounts := usecases.NewListAccounts(accountRepo, movementRepo)
	reportBalance := usecases.NewReportAccountBalance(accountRepo, movementRepo)
	listCurrencies := usecases.NewListCurrencies(currencyRepo)
	addCurrency := usecases.NewAddCurrency(currencyRepo)
	transferBetweenAccounts := usecases.NewTransferBetweenAccounts(movementRepo, accountRepo)
	cancelTransfer := usecases.NewCancelTransfer(movementRepo, syncService)
	setExchangeRate := usecases.NewSetExchangeRate(exchangeRateRepo, currencyRepo)
	listExchangeRates := usecases.NewListExchangeRates(exchangeRateRepo)
	deleteExchangeRate := usecases.NewDeleteExchangeRate(exchangeRateRepo)

	movementHandler := handlers.NewMovementHandler(
		createMovement,
		createPurchase,
		getMovement,
		listMovements,
		updateMovement,
		cancelMovement,
		cancelPurchase,
		getCashflow,
		syncService,
		defaultUserID,
		defaultCurrency,
		log,
	)
	accountHandler := handlers.NewAccountHandler(createAccount, listAccounts, reportBalance, defaultUserID, log)
	currencyHandler := handlers.NewCurrencyHandler(listCurrencies, addCurrency, log)
	transferHandler := handlers.NewTransferHandler(transferBetweenAccounts, cancelTransfer, defaultUserID, log)
	exchangeRateHandler := handlers.NewExchangeRateHandler(setExchangeRate, listExchangeRates, deleteExchangeRate, defaultUserID, log)

	router := api.NewRouter(movementHandler, accountHandler, currencyHandler, transferHandler, exchangeRateHandler, corsAllowedOrigin)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	syncService.Start(ctx, syncInterval)

	dbDescription := dbPath
	if dbDriver == "postgres" {
		dbDescription = "postgres"
	}
	addr := ":" + port
	log.Info("financial-tracker API listening on %s (db driver %s at %s, syncing to ledger-service at %s every %s)", addr, dbDriver, dbDescription, ledgerServiceURL, syncInterval)
	log.Info("endpoints: POST /movements | GET /movements | PATCH /movements/{id} | POST /movements/{id}/cancel | POST /credit-card-purchases/{id}/cancel | POST /sync | GET /categories | GET /cashflow | GET|POST /accounts | POST /accounts/{id}/balance | GET|POST /currencies | POST /transfers | POST /transfers/{id}/cancel | GET|POST /exchange-rates | DELETE /exchange-rates/{id}")

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
