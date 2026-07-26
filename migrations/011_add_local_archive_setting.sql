-- local_archive_enabled is BACK-15's "no cloud" tier toggle: a user who
-- opts in gets a client-side-encrypted export/import flow instead of (or
-- alongside) whatever server-side storage exists. Kept as its own table
-- rather than folded into a shared user_settings row, since BACK-13's
-- user_settings hasn't landed in main yet (soft dep — see the ticket).
-- Stored as INTEGER 0/1, like every other flag in this schema (SQLite has
-- no native boolean type); the Go layer converts explicitly, matching
-- this codebase's existing convention for scanned columns.
CREATE TABLE IF NOT EXISTS user_local_archive_settings (
    user_id                TEXT    PRIMARY KEY,
    local_archive_enabled  INTEGER NOT NULL DEFAULT 0 CHECK (local_archive_enabled IN (0, 1)),
    updated_at             TEXT    NOT NULL
);
