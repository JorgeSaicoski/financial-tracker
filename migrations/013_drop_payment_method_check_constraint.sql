-- BACK-17 turned payment_method into a free-form, per-user-registry-
-- resolved string (see migrations/012_create_payment_methods_table.sql)
-- instead of a fixed enum — but 001_create_movements_table.sql's CHECK
-- constraint still only allowed the old fixed list ('pix' included), so
-- any payment method outside it (including brand-new implicitly-
-- registered ones) was silently rejected by the database regardless of
-- what the application layer allowed.
--
-- SQLite can't drop a CHECK constraint in place, so the table is rebuilt
-- without it, following the same documented 12-step ALTER TABLE
-- procedure 009_add_local_sync_status.sql already used for sync_status.
-- This migration sorts right after 013_drop_category_check_constraints.sql
-- (same "013" prefix, "category" < "payment_method"), which already
-- rebuilt this exact table once to drop *its* CHECK constraint — every
-- column that rebuild carried over (avoidability_override_percent,
-- recurring_rule_id, the now CHECK-free category column) must be carried
-- over here too, or this second same-migration-number rebuild would
-- silently drop them right back out from under it.
CREATE TABLE movements_new (
    id                            TEXT PRIMARY KEY,
    user_id                       TEXT    NOT NULL,
    amount                        INTEGER NOT NULL,
    currency                      TEXT    NOT NULL,
    description                   TEXT,
    category                      TEXT    NOT NULL DEFAULT 'other',
    payment_method                TEXT    NOT NULL DEFAULT 'other',
    credit_card_purchase_id       TEXT    REFERENCES credit_card_purchases(id),
    installment_number            INTEGER,
    status                        TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active','voided')),
    cancels_movement_id           TEXT    REFERENCES movements_new(id),
    reversed_by_movement_id       TEXT    REFERENCES movements_new(id),
    timestamp                     TEXT    NOT NULL,
    sync_status                   TEXT    NOT NULL DEFAULT 'pending' CHECK (sync_status IN ('pending','synced','failed','local')),
    ledger_transaction_id         TEXT,
    sync_attempts                 INTEGER NOT NULL DEFAULT 0,
    last_sync_error               TEXT,
    last_sync_attempt_at          TEXT,
    synced_at                     TEXT,
    created_at                    TEXT    NOT NULL,
    account_id                    TEXT    REFERENCES accounts(id),
    transfer_id                   TEXT,
    avoidability_override_percent INTEGER,
    recurring_rule_id             TEXT    REFERENCES recurring_rules(id)
);

INSERT INTO movements_new (
    id, user_id, amount, currency, description, category, payment_method,
    credit_card_purchase_id, installment_number, status,
    cancels_movement_id, reversed_by_movement_id, timestamp, sync_status,
    ledger_transaction_id, sync_attempts, last_sync_error,
    last_sync_attempt_at, synced_at, created_at, account_id, transfer_id,
    avoidability_override_percent, recurring_rule_id
)
SELECT
    id, user_id, amount, currency, description, category, payment_method,
    credit_card_purchase_id, installment_number, status,
    cancels_movement_id, reversed_by_movement_id, timestamp, sync_status,
    ledger_transaction_id, sync_attempts, last_sync_error,
    last_sync_attempt_at, synced_at, created_at, account_id, transfer_id,
    avoidability_override_percent, recurring_rule_id
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
