package services

import "context"

// FieldCryptor encrypts/decrypts individual free-text field values at
// rest (BACK-16) — movements.description, accounts.name in the Postgres
// ("cloud") backend. Implemented by infrastructure/crypto with per-user
// envelope encryption (AES-256-GCM); application code and domain
// entities never see ciphertext, only plaintext in and out. Encrypt and
// Decrypt of an empty string both return "" — callers don't need a
// special case to preserve the "empty means absent" contract movements
// and accounts already rely on.
type FieldCryptor interface {
	Encrypt(ctx context.Context, userID, plaintext string) (ciphertext string, err error)
	Decrypt(ctx context.Context, userID, ciphertext string) (plaintext string, err error)
}

// LedgerPseudonymizer resolves the pseudonymous identity financial-tracker
// presents to ledger-service in place of the real user id and plain
// currency code (BACK-16), so the append-only audit trail stays fully
// auditable without being attributable to a real person. Implemented by
// infrastructure/crypto, consumed at exactly one point:
// infrastructure/ledgerservice.gateway.Publish — nothing upstream of
// Publish needs to know pseudonymization exists.
type LedgerPseudonymizer interface {
	// PseudonymFor returns userID's stable, non-reversible pseudonym
	// UUID, minting and persisting one on first call. Never derived from
	// the real user id.
	PseudonymFor(ctx context.Context, userID string) (string, error)
	// TokenizeCurrency returns a deterministic HMAC token for
	// currencyCode: the same (userID, currencyCode) pair always produces
	// the same token, so ledger-service's own consistency checks still
	// work, without either the real user id or the plain currency code
	// ever crossing the wire. Amounts are never tokenized or hidden —
	// only who and what currency.
	TokenizeCurrency(ctx context.Context, userID, currencyCode string) (string, error)
}
