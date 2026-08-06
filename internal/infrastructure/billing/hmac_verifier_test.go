package billing

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(secret, payload []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestHMACWebhookVerifierAcceptsValidSignature(t *testing.T) {
	secret := []byte("test-secret-at-least-16-bytes")
	payload := []byte(`{"user_id":"u1","status":"active"}`)
	v := NewHMACWebhookVerifier(secret)

	if err := v.Verify(payload, sign(secret, payload)); err != nil {
		t.Errorf("unexpected error for a valid signature: %v", err)
	}
}

func TestHMACWebhookVerifierRejectsWrongSecret(t *testing.T) {
	payload := []byte(`{"user_id":"u1","status":"active"}`)
	v := NewHMACWebhookVerifier([]byte("real-secret-at-least-16-bytes"))

	if err := v.Verify(payload, sign([]byte("wrong-secret-at-least-16-bytes"), payload)); err == nil {
		t.Error("want an error for a signature computed with the wrong secret")
	}
}

func TestHMACWebhookVerifierRejectsTamperedPayload(t *testing.T) {
	secret := []byte("test-secret-at-least-16-bytes")
	v := NewHMACWebhookVerifier(secret)

	sig := sign(secret, []byte(`{"user_id":"u1","status":"active"}`))
	if err := v.Verify([]byte(`{"user_id":"u1","status":"canceled"}`), sig); err == nil {
		t.Error("want an error when the payload doesn't match the signature")
	}
}

func TestHMACWebhookVerifierRejectsMalformedSignature(t *testing.T) {
	v := NewHMACWebhookVerifier([]byte("test-secret-at-least-16-bytes"))
	if err := v.Verify([]byte("payload"), "not-hex!!"); err == nil {
		t.Error("want an error for a non-hex signature")
	}
}
