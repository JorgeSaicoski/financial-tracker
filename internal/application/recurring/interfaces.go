package recurring

import (
	"context"
	"time"
)

// Summary is the result of one generation pass.
type Summary struct {
	RulesProcessed     int
	MovementsGenerated int
	Errors             int
}

// Service is everything main.go needs from the recurring-generation
// engine: RunPass for a manual trigger, Start for the background-loop
// bootstrap only the composition root calls — the same shape as
// application/sync.Service.
type Service interface {
	// RunPass generates every due movement across every active rule (all
	// users, one pass) and returns what happened. A single rule's failure
	// is logged and counted, not fatal to the rest of the pass.
	RunPass(ctx context.Context) Summary
	// Start runs a generation pass every interval until ctx is cancelled.
	Start(ctx context.Context, interval time.Duration)
}
