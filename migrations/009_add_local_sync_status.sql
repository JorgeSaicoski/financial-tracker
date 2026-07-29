-- BACK-13: movements created while a user's ledger sync is off get the
-- new terminal 'local' sync_status instead of sitting in 'pending'
-- forever (they were never going to sync in the first place, so
-- 'pending' would be a lie about pending work that will never happen).
--
-- SQLite can't ALTER a CHECK constraint in place, so this rebuilds the
-- movements table with 'local' added to sync_status's allowed values,
-- following SQLite's documented 12-step ALTER TABLE procedure. Foreign
-- key enforcement stays on the whole time (it can't be toggled mid
-- transaction, and Migrate always runs each file inside one) — that's
-- fine here because the single INSERT...SELECT below copies every row,
-- including the self-referencing cancels_movement_id/
-- reversed_by_movement_id columns, in one statement, so those references
-- resolve against rows already present in movements_new by the time
-- SQLite checks them. account_id (added by migration 004, referencing
-- accounts(id), which already exists by this point), transfer_id
-- (added by migration 005), and recurring_rule_id (added by migration
-- 008, referencing recurring_rules(id), which also already exists by
-- this point) all need to be carried over too, since this rebuilds the
-- *whole* table, not just the column 001 originally shipped.
CREATE TABLE movements_new (
    id                      TEXT PRIMARY KEY,
    user_id                 TEXT    NOT NULL,
    amount                  INTEGER NOT NULL,
    currency                TEXT    NOT NULL,
    description             TEXT,
    category                TEXT    NOT NULL DEFAULT 'other' CHECK (category IN (
        'food','transport','housing','utilities','health','entertainment',
        'shopping','education','income','transfer','other')),
    payment_method          TEXT    NOT NULL DEFAULT 'other' CHECK (payment_method IN (
        'cash','debit_card','credit_card','pix','bank_transfer','other')),
    credit_card_purchase_id TEXT    REFERENCES credit_card_purchases(id),
    installment_number      INTEGER,
    status                  TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active','voided')),
    cancels_movement_id     TEXT    REFERENCES movements_new(id),
    reversed_by_movement_id TEXT    REFERENCES movements_new(id),
    timestamp               TEXT    NOT NULL,
    sync_status             TEXT    NOT NULL DEFAULT 'pending' CHECK (sync_status IN ('pending','synced','failed','local')),
    ledger_transaction_id   TEXT,
    sync_attempts           INTEGER NOT NULL DEFAULT 0,
    last_sync_error         TEXT,
    last_sync_attempt_at    TEXT,
    synced_at               TEXT,
    created_at              TEXT    NOT NULL,
    account_id              TEXT    REFERENCES accounts(id),
    transfer_id             TEXT,
    recurring_rule_id       TEXT    REFERENCES recurring_rules(id)
);

INSERT INTO movements_new (
    id, user_id, amount, currency, description, category, payment_method,
    credit_card_purchase_id, installment_number, status,
    cancels_movement_id, reversed_by_movement_id, timestamp, sync_status,
    ledger_transaction_id, sync_attempts, last_sync_error,
    last_sync_attempt_at, synced_at, created_at, account_id, transfer_id,
    recurring_rule_id
)
SELECT
    id, user_id, amount, currency, description, category, payment_method,
    credit_card_purchase_id, installment_number, status,
    cancels_movement_id, reversed_by_movement_id, timestamp, sync_status,
    ledger_transaction_id, sync_attempts, last_sync_error,
    last_sync_attempt_at, synced_at, created_at, account_id, transfer_id,
    recurring_rule_id
FROM movements;

DROP TABLE movements;

ALTER TABLE movements_new RENAME TO movements;

CREATE INDEX IF NOT EXISTS idx_movements_user_timestamp
    ON movements (user_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_movements_pending_sync
    ON movements (sync_status, timestamp) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_movements_purchase
    ON movements (credit_card_purchase_id) WHERE credit_card_purchase_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_movements_transfer
    ON movements (transfer_id) WHERE transfer_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_movements_account
    ON movements (account_id) WHERE account_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_movements_recurring_rule
    ON movements (recurring_rule_id) WHERE recurring_rule_id IS NOT NULL;
