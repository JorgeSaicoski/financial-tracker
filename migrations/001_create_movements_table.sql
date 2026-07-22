-- movements is financial-tracker's local source of truth. ledger-service
-- is synced to in the background (sync_* columns track that).
-- All timestamps are stored as RFC 3339 UTC text.
CREATE TABLE IF NOT EXISTS movements (
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
    cancels_movement_id     TEXT    REFERENCES movements(id),
    reversed_by_movement_id TEXT    REFERENCES movements(id),
    timestamp               TEXT    NOT NULL,
    sync_status             TEXT    NOT NULL DEFAULT 'pending' CHECK (sync_status IN ('pending','synced','failed')),
    ledger_transaction_id   TEXT,
    sync_attempts           INTEGER NOT NULL DEFAULT 0,
    last_sync_error         TEXT,
    last_sync_attempt_at    TEXT,
    synced_at               TEXT,
    created_at              TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_movements_user_timestamp
    ON movements (user_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_movements_pending_sync
    ON movements (sync_status, timestamp) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_movements_purchase
    ON movements (credit_card_purchase_id) WHERE credit_card_purchase_id IS NOT NULL;
