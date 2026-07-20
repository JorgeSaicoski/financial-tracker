package main

import (
	"net/http"
	"os"

	"github.com/JorgeSaicoski/financial-tracker/application/usecases"
	"github.com/JorgeSaicoski/financial-tracker/infrastructure/ledgerservice"
	"github.com/JorgeSaicoski/financial-tracker/interfaces/api"
	"github.com/JorgeSaicoski/financial-tracker/interfaces/api/handlers"
	applogger "github.com/JorgeSaicoski/financial-tracker/pkg/logger"
)

func main() {
	ledgerServiceURL := os.Getenv("LEDGER_SERVICE_URL")
	if ledgerServiceURL == "" {
		ledgerServiceURL = "http://localhost:8080"
	}

	// MVP has no auth yet: every request without an explicit user_id is
	// attributed to this fixed dev user. Must be a lowercase UUID, since
	// that's what ledger-service's validator requires.
	defaultUserID := os.Getenv("DEFAULT_USER_ID")
	if defaultUserID == "" {
		defaultUserID = "00000000-0000-0000-0000-000000000001"
	}

	defaultCurrency := os.Getenv("DEFAULT_CURRENCY")
	if defaultCurrency == "" {
		defaultCurrency = "usd"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	log := applogger.New()

	// Infrastructure: today the only MovementRepository implementation is
	// ledger-service over HTTP. Swapping to Postgres later means writing
	// infrastructure/postgresql.MovementRepository and changing only the
	// two lines below.
	ledgerClient := ledgerservice.NewClient(ledgerServiceURL)
	movementRepo := ledgerservice.NewMovementRepository(ledgerClient)

	createMovement := usecases.NewCreateMovement(movementRepo)
	getMovement := usecases.NewGetMovement(movementRepo)
	listMovements := usecases.NewListMovements(movementRepo)

	movementHandler := handlers.NewMovementHandler(
		createMovement,
		getMovement,
		listMovements,
		defaultUserID,
		defaultCurrency,
		log,
	)

	router := api.NewRouter(movementHandler)

	addr := ":" + port
	log.Info("financial-tracker API listening on %s (ledger-service at %s)", addr, ledgerServiceURL)
	log.Info("endpoints: POST /movements | GET /movements?id=<uuid> | GET /movements?user_id=<uuid>")

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Error("server failed: %v", err)
		os.Exit(1)
	}
}
