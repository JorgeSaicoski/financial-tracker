-- user_settings (BACK-13) separates entitlement (what a user is *allowed*
-- to use, operator/billing-controlled) from preference (what they've
-- *chosen* to enable). Effective capability is entitled AND enabled.
-- Absence of a row means "everything true" (see the repository contract)
-- so existing users need no backfill; a row is only inserted lazily on
-- first PATCH /settings write.
CREATE TABLE IF NOT EXISTS user_settings (
    user_id                 TEXT PRIMARY KEY,
    ledger_sync_entitled    BOOLEAN     NOT NULL DEFAULT TRUE,
    ledger_sync_enabled     BOOLEAN     NOT NULL DEFAULT TRUE,
    cloud_storage_entitled  BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at              TIMESTAMPTZ NOT NULL,
    updated_at              TIMESTAMPTZ NOT NULL
);
