// Package billing implements BACK-19's payment-webhook verification. It
// deliberately contains no payment-provider SDK: like
// infrastructure/ledgerservice's hand-rolled HTTP client, this is a
// plain crypto/hmac check behind the application layer's
// services.PaymentWebhookVerifier port, so swapping the real check for a
// specific provider's own scheme later (e.g. Stripe's timestamped
// `Stripe-Signature` header) means changing this package only.
package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
)

type hmacWebhookVerifier struct {
	secret []byte
}

// NewHMACWebhookVerifier returns a services.PaymentWebhookVerifier that
// checks signature is the hex-encoded HMAC-SHA256 of payload under
// secret — the simplest authenticity check that still requires the
// caller to know a shared secret, matching BACK-16's existing
// crypto/hmac usage for the ledger pseudonym token.
func NewHMACWebhookVerifier(secret []byte) services.PaymentWebhookVerifier {
	return &hmacWebhookVerifier{secret: secret}
}

func (v *hmacWebhookVerifier) Verify(payload []byte, signature string) error {
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("billing: signature is not valid hex: %w", err)
	}

	mac := hmac.New(sha256.New, v.secret)
	mac.Write(payload)
	expected := mac.Sum(nil)

	if !hmac.Equal(expected, provided) {
		return fmt.Errorf("billing: signature does not match payload")
	}
	return nil
}
