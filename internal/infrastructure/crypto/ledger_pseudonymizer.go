package crypto

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/JorgeSaicoski/financial-tracker/internal/application/dto"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/repositories"
	"github.com/JorgeSaicoski/financial-tracker/internal/application/services"
	apperrors "github.com/JorgeSaicoski/financial-tracker/internal/pkg/errors"
	"github.com/JorgeSaicoski/financial-tracker/internal/pkg/id"
)

type ledgerPseudonymizer struct {
	hmacKey        []byte
	pseudonyms     repositories.LedgerPseudonymRepository
	pseudonymCache sync.Map // userID string -> pseudonym string
}

// NewLedgerPseudonymizer returns a services.LedgerPseudonymizer
// implementing BACK-16's pseudonymous ledger sync: a random, persisted,
// non-reversible per-user UUID in place of the real user id, plus a
// deterministic HMAC-SHA256 token (keyed by hmacKey, never stored) in
// place of the plain currency code.
func NewLedgerPseudonymizer(hmacKey []byte, pseudonyms repositories.LedgerPseudonymRepository) services.LedgerPseudonymizer {
	return &ledgerPseudonymizer{hmacKey: hmacKey, pseudonyms: pseudonyms}
}

func (p *ledgerPseudonymizer) PseudonymFor(ctx context.Context, userID string) (string, error) {
	if cached, ok := p.pseudonymCache.Load(userID); ok {
		return cached.(string), nil
	}

	row, err := p.pseudonyms.Get(ctx, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		row, err = p.pseudonyms.Create(ctx, &dto.LedgerPseudonymDTO{
			UserID:      userID,
			PseudonymID: id.NewUUID(),
			CreatedAt:   time.Now().UTC(),
		})
	}
	if err != nil {
		return "", fmt.Errorf("crypto: resolve ledger pseudonym for user %s: %w", userID, err)
	}

	p.pseudonymCache.Store(userID, row.PseudonymID)
	return row.PseudonymID, nil
}

// TokenizeCurrency is deterministic given (hmacKey, userID,
// currencyCode) — it needs no storage, unlike PseudonymFor, since HMAC
// itself already guarantees "same input -> same output" without
// remembering anything.
func (p *ledgerPseudonymizer) TokenizeCurrency(_ context.Context, userID, currencyCode string) (string, error) {
	mac := hmac.New(sha256.New, p.hmacKey)
	mac.Write([]byte(userID))
	mac.Write([]byte{0}) // separator: avoids "ab"+"c" colliding with "a"+"bc"
	mac.Write([]byte(currencyCode))
	sum := mac.Sum(nil)
	return "c_" + hex.EncodeToString(sum)[:16], nil
}
