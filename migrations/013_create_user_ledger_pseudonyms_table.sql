-- user_ledger_pseudonyms (BACK-16) maps a user to the random,
-- non-reversible UUID sent to ledger-service in their place once
-- ledger_sync_enabled is on — never derived from the real user id, so it
-- can't be reversed. Minted lazily on first sync; never backfilled.
-- Movements already synced under the real user id before this landed
-- stay as they are in ledger-service's append-only log.
CREATE TABLE IF NOT EXISTS user_ledger_pseudonyms (
    user_id      TEXT PRIMARY KEY,
    pseudonym_id TEXT NOT NULL UNIQUE,
    created_at   TEXT NOT NULL
);
