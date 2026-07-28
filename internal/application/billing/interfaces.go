// Package billing runs BACK-19's grace-period sweep: a background pass
// that flips cloud_storage_entitled to false for subscriptions still
// past_due or canceled once their grace period has elapsed. Only the
// active -> true entitlement change happens synchronously, in
// usecases.ProcessBillingWebhookUseCase — losing entitlement (whether
// from a late payment or an explicit cancellation) always waits for this
// sweep instead, so "a late card shouldn't cut off access instantly"
// also covers "cancelling doesn't cut you off mid-webhook-request."
package billing

import (
	"context"
	"time"
)

// Summary is the result of one sweep pass.
type Summary struct {
	SubscriptionsChecked int
	EntitlementsLapsed   int
	Errors               int
}

// Service is everything main.go needs from the grace-period sweep:
// RunPass for a manual trigger, Start for the background-loop bootstrap
// only the composition root calls — the same shape as
// application/recurring.Service.
type Service interface {
	// RunPass flips cloud_storage_entitled to false for every past_due or
	// canceled subscription whose grace period has elapsed as of now. A
	// single user's failure is logged and counted, not fatal to the rest
	// of the pass.
	RunPass(ctx context.Context) Summary
	// Start runs a sweep pass every interval until ctx is cancelled.
	Start(ctx context.Context, interval time.Duration)
}
