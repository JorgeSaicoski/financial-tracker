-- users are the local identity every account/movement/etc. is scoped to.
-- Provisioned automatically on first successful auth (EnsureUserUseCase,
-- BACK-02) rather than through a signup endpoint — the identity provider
-- (Authentik today) already owns registration; this just mirrors what it
-- asserts about the caller. No foreign key from accounts/movements.user_id
-- to this table yet, matching how those columns are already plain TEXT
-- with no REFERENCES — adding one is a separate migration once every
-- existing user_id in those tables is guaranteed to have a row here.
-- Whether this user's movements sync to ledger-service lives in
-- user_settings (BACK-13, migration 010), not here.
CREATE TABLE IF NOT EXISTS users (
    id           TEXT        PRIMARY KEY,
    provider     TEXT        NOT NULL DEFAULT '',
    external_id  TEXT        NOT NULL DEFAULT '',
    email        TEXT        NOT NULL DEFAULT '',
    display_name TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
