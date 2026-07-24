package sync

import (
	"context"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
)

// Service is everything main.go needs from the sync engine: the two
// application-defined ports it implements (services.SyncTrigger,
// services.SyncRunner) plus Start, the background-loop bootstrap that
// only the composition root calls. Consolidated here, separate from the
// concrete implementation in service.go, per this workspace's
// CleanExampleGo-derived architecture rules.
type Service interface {
	services.SyncTrigger
	services.SyncRunner
	// RunPass syncs due movements, skipping rows attempted within the
	// retry cooldown — this is what the background ticker calls.
	RunPass(ctx context.Context) services.Summary
	// Start runs a sync pass every interval until ctx is cancelled.
	Start(ctx context.Context, interval time.Duration)
}
