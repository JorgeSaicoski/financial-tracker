-- user_data_keys (BACK-16) holds each user's envelope-encryption data
-- key: a random AES-256 key, generated once and wrapped (AES-256-GCM)
-- under the server's master key (ENCRYPTION_MASTER_KEY), never stored
-- unencrypted. Postgres-only in practice (SQLite/standalone deployments
-- never construct a FieldCryptor — see cmd/api/main.go) but the table
-- exists in both dialects for schema parity with every other table.
-- Minted lazily on first encrypted write; never backfilled. Key rotation
-- is out of scope for v1.
CREATE TABLE IF NOT EXISTS user_data_keys (
    user_id     TEXT PRIMARY KEY,
    wrapped_key TEXT NOT NULL,
    created_at  TEXT NOT NULL
);
