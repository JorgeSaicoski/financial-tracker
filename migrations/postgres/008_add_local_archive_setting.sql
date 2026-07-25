-- Mirrors migrations/008_add_local_archive_setting.sql — see its comment
-- for why this is a standalone table rather than a shared user_settings
-- row. Postgres has a native boolean type, unlike SQLite's INTEGER 0/1.
CREATE TABLE IF NOT EXISTS user_local_archive_settings (
    user_id                TEXT        PRIMARY KEY,
    local_archive_enabled  BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at             TIMESTAMPTZ NOT NULL
);
