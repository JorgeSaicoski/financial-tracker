-- BACK-13: movements created while a user's ledger sync is off get the
-- new terminal 'local' sync_status instead of sitting in 'pending'
-- forever — see migrations/008_add_local_sync_status.sql (the SQLite
-- version) for the full rationale. Postgres can alter a CHECK constraint
-- in place, unlike SQLite, so this is a plain drop+recreate; the
-- constraint's default auto-generated name follows Postgres's
-- <table>_<column>_check convention for an unnamed inline CHECK.
ALTER TABLE movements DROP CONSTRAINT movements_sync_status_check;
ALTER TABLE movements ADD CONSTRAINT movements_sync_status_check
    CHECK (sync_status IN ('pending', 'synced', 'failed', 'local'));
