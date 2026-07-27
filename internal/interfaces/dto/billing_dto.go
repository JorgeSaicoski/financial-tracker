package dto

import "time"

// BillingWebhookRequest is POST /billing/webhook's body — already
// translated into financial-tracker's provider-agnostic subscription
// shape (see services.PaymentWebhookVerifier's doc comment for why this
// isn't a specific provider's own event schema). Authenticity comes from
// the request's signature header, checked before this body is even
// decoded (see the handler) — never from anything in the body itself.
type BillingWebhookRequest struct {
	UserID                 string    `json:"user_id"`
	Provider               string    `json:"provider"`
	ProviderSubscriptionID string    `json:"provider_subscription_id"`
	Status                 string    `json:"status"`
	CurrentPeriodEnd       time.Time `json:"current_period_end"`
}

// BillingWebhookResponse confirms what the webhook actually recorded.
type BillingWebhookResponse struct {
	UserID           string    `json:"user_id"`
	Status           string    `json:"status"`
	CurrentPeriodEnd time.Time `json:"current_period_end"`
}

// BillingPlanResponse is GET /billing/plan's response. Currency is the
// currency Amount is actually expressed in — it may differ from what the
// caller requested (see usecases.BillingPlanView's doc comment for the
// fallback rule), so a client must read it back rather than assuming it
// echoes the request.
type BillingPlanResponse struct {
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}
