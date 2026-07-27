package dto

import "time"

// UserDataKeyDTO is BACK-16's per-user envelope-encryption key: a random
// AES-256 data key, generated once and wrapped (AES-256-GCM) under the
// server's master key (ENCRYPTION_MASTER_KEY) so the raw key is never
// stored unencrypted. Minted lazily on first encrypted write for a user;
// never backfilled. Key rotation is out of scope for v1 — see BACK-16.
type UserDataKeyDTO struct {
	UserID     string
	WrappedKey string // base64(nonce || AES-256-GCM(masterKey, rawDataKey))
	CreatedAt  time.Time
}
