package crypto

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
)

type fieldCryptor struct {
	masterKey []byte
	keys      repositories.UserDataKeyRepository

	// keyCache holds each user's *unwrapped* data key for the life of
	// this process, keyed by user id. Without it, every single
	// Encrypt/Decrypt call (e.g. once per row in ListByUser) would round
	// -trip to the database just to re-fetch and re-unwrap the same
	// key. This is safe under BACK-16's own threat model: it protects
	// against a stolen disk/DB dump, not against a compromised running
	// server — a live process that can decrypt on request is exactly as
	// capable of decrypting with or without this cache. Data keys never
	// rotate in v1 (see BACK-16), so nothing ever needs to invalidate an
	// entry.
	keyCache sync.Map // userID string -> []byte unwrapped key
}

// NewFieldCryptor returns a services.FieldCryptor implementing BACK-16's
// envelope encryption: each user gets one random AES-256 data key,
// generated on first use and persisted wrapped (AES-256-GCM) under
// masterKey. Field values are then sealed with that per-user key, so a
// raw DB dump is unreadable without masterKey, while the running server
// can still decrypt on behalf of the field's own owner.
func NewFieldCryptor(masterKey []byte, keys repositories.UserDataKeyRepository) services.FieldCryptor {
	return &fieldCryptor{masterKey: masterKey, keys: keys}
}

func (c *fieldCryptor) Encrypt(ctx context.Context, userID, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	dataKey, err := c.dataKey(ctx, userID)
	if err != nil {
		return "", err
	}
	ciphertext, err := seal(dataKey, []byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("crypto: encrypt field for user %s: %w", userID, err)
	}
	return ciphertext, nil
}

func (c *fieldCryptor) Decrypt(ctx context.Context, userID, ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	dataKey, err := c.dataKey(ctx, userID)
	if err != nil {
		return "", err
	}
	plaintext, err := open(dataKey, ciphertext)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypt field for user %s: %w", userID, err)
	}
	return string(plaintext), nil
}

// dataKey returns userID's unwrapped per-user data key, generating and
// persisting one (wrapped under masterKey) on first use.
func (c *fieldCryptor) dataKey(ctx context.Context, userID string) ([]byte, error) {
	if cached, ok := c.keyCache.Load(userID); ok {
		return cached.([]byte), nil
	}

	row, err := c.keys.Get(ctx, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("crypto: generate data key: %w", err)
		}
		wrapped, err := seal(c.masterKey, raw)
		if err != nil {
			return nil, fmt.Errorf("crypto: wrap data key: %w", err)
		}
		row, err = c.keys.Create(ctx, &dto.UserDataKeyDTO{
			UserID:     userID,
			WrappedKey: wrapped,
			CreatedAt:  time.Now().UTC(),
		})
		if err != nil {
			return nil, fmt.Errorf("crypto: persist data key: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("crypto: get data key: %w", err)
	}

	unwrapped, err := open(c.masterKey, row.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("crypto: unwrap data key for user %s: %w", userID, err)
	}
	c.keyCache.Store(userID, unwrapped)
	return unwrapped, nil
}
