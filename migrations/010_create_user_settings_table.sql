-- user_settings (BACK-13) separates entitlement (what a user is *allowed*
-- to use, operator/billing-controlled) from preference (what they've
-- *chosen* to enable). Effective capability is entitled AND enabled.
-- Absence of a row means "everything true" (see the repository contract)
-- so existing users need no backfill; a row is only inserted lazily on
-- first PATCH /settings write.
CREATE TABLE IF NOT EXISTS user_settings (
    user_id                 TEXT PRIMARY KEY,
    ledger_sync_entitled    INTEGER NOT NULL DEFAULT 1,
    ledger_sync_enabled     INTEGER NOT NULL DEFAULT 1,
    cloud_storage_entitled  INTEGER NOT NULL DEFAULT 1,
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL
);
