package services

// PaymentWebhookVerifier is the port POST /billing/webhook needs from the
// outside world: prove a webhook body actually came from the configured
// payment provider before any of it is trusted (BACK-19). This
// deliberately doesn't model Stripe's (or any specific provider's) own
// signature scheme — the provider is a build-time choice per the ticket,
// not something the application layer should know about. A real
// integration adapts whatever the provider actually sends (e.g. Stripe's
// timestamped `Stripe-Signature` header) into a call to Verify here,
// the same way infrastructure/authentik is the only package that knows
// OIDC/JWT/JWKS exist for identity verification.
type PaymentWebhookVerifier interface {
	// Verify reports an error if signature doesn't authenticate payload.
	Verify(payload []byte, signature string) error
}
