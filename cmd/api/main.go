package main

import (
	"context"
	"net/http"
	"os"
	"time"

	syncapp "github.com/JorgeSaicoski/financial-tracker/application/sync"
	"github.com/JorgeSaicoski/financial-tracker/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/infrastructure/ledgerservice"
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
	dbPath := envOr("DB_PATH", "./data/financial-tracker.db")

	log := applogger.New()

	syncInterval := durationEnvOr(log, "SYNC_INTERVAL", 30*time.Second)
	retryCooldown := durationEnvOr(log, "SYNC_RETRY_COOLDOWN", 60*time.Second)

	// Infrastructure: the local SQLite database is the source of truth;
	// ledger-service is only reached by the background sync, so requests
	// keep working while it's down.
	db, err := sqlite.Open(dbPath)
	if err != nil {
		log.Error("opening database failed: %v", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := sqlite.Migrate(db); err != nil {
		log.Error("migrating database failed: %v", err)
		os.Exit(1)
	}

	movementRepo := sqlite.NewMovementRepository(db)
	purchaseRepo := sqlite.NewCreditCardPurchaseRepository(db)

	ledgerClient := ledgerservice.NewClient(ledgerServiceURL)
	ledgerGateway := ledgerservice.NewLedgerGateway(ledgerClient)
	syncService := syncapp.NewService(movementRepo, ledgerGateway, log, retryCooldown)

	createMovement := usecases.NewCreateMovement(movementRepo)
	createPurchase := usecases.NewCreateCreditCardPurchase(purchaseRepo)
	getMovement := usecases.NewGetMovement(movementRepo)
	listMovements := usecases.NewListMovements(movementRepo)
	cancelMovement := usecases.NewCancelMovement(movementRepo, syncService)
	cancelPurchase := usecases.NewCancelCreditCardPurchase(purchaseRepo, movementRepo, syncService)

	movementHandler := handlers.NewMovementHandler(
		createMovement,
		createPurchase,
		getMovement,
		listMovements,
		cancelMovement,
		cancelPurchase,
		syncService,
		defaultUserID,
		defaultCurrency,
		log,
	)

	router := api.NewRouter(movementHandler)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	syncService.Start(ctx, syncInterval)

	addr := ":" + port
	log.Info("financial-tracker API listening on %s (db %s, syncing to ledger-service at %s every %s)", addr, dbPath, ledgerServiceURL, syncInterval)
	log.Info("endpoints: POST /movements | GET /movements | POST /movements/{id}/cancel | POST /credit-card-purchases/{id}/cancel | POST /sync | GET /categories")

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
