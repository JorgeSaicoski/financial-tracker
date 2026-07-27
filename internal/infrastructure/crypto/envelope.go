// Package crypto implements BACK-16's server-held-master-key envelope
// encryption and ledger-service pseudonymization. It is the only place
// in financial-tracker that touches raw key material — every other layer
// depends on the application/services.FieldCryptor and
// LedgerPseudonymizer ports, never on this package directly.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// seal encrypts plaintext with AES-256-GCM under key (must be 32 bytes),
// returning base64(nonce || ciphertext || tag) — GCM's Seal appends the
// authentication tag itself, so nothing else needs tracking alongside it.
// Used both to wrap a per-user data key under the master key and to
// encrypt a field value under a per-user data key.
func seal(key, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("crypto: read nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// open reverses seal. Returns an error if key is wrong, encoded is
// malformed, or the authentication tag doesn't match (tampered/corrupt
// ciphertext).
func open(key []byte, encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("crypto: decode ciphertext: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return nil, fmt.Errorf("crypto: ciphertext too short")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("crypto: decrypt: %w", err)
	}
	return plaintext, nil
}

// ParseMasterKey decodes ENCRYPTION_MASTER_KEY (base64, must be exactly
// 32 bytes decoded — AES-256) — failing loud on anything else, since a
// short/malformed key would silently weaken every field it protects.
// Generate with: openssl rand -base64 32
func ParseMasterKey(b64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("master key is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("master key must decode to exactly 32 bytes (AES-256), got %d", len(key))
	}
	return key, nil
}

// ParseHMACKey decodes LEDGER_HMAC_KEY (base64, at least 16 bytes
// decoded — HMAC-SHA256 accepts any length, but a short key would
// weaken the pseudonymization it's used for). Generate with:
// openssl rand -base64 32
func ParseHMACKey(b64 string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("HMAC key is not valid base64: %w", err)
	}
	if len(key) < 16 {
		return nil, fmt.Errorf("HMAC key must decode to at least 16 bytes, got %d", len(key))
	}
	return key, nil
}
