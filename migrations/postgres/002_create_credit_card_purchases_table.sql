-- A purchase is a grouping record only. The actual money movements are the
-- rows in movements with credit_card_purchase_id set.
CREATE TABLE IF NOT EXISTS credit_card_purchases (
    id                TEXT        PRIMARY KEY,
    user_id           TEXT        NOT NULL,
    description       TEXT,
    category          TEXT        NOT NULL DEFAULT 'other' CHECK (category IN (
        'food','transport','housing','utilities','health','entertainment',
        'shopping','education','income','transfer','other')),
    total_amount      BIGINT      NOT NULL,
    currency          TEXT        NOT NULL,
    installment_count INTEGER     NOT NULL,
    purchase_date     TIMESTAMPTZ NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active','cancelled')),
    created_at        TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_credit_card_purchases_user
    ON credit_card_purchases (user_id, purchase_date DESC);

-- Deferred from 001: movements.credit_card_purchase_id can only reference
-- this table now that it exists.
ALTER TABLE movements
    ADD CONSTRAINT fk_movements_credit_card_purchase
    FOREIGN KEY (credit_card_purchase_id) REFERENCES credit_card_purchases(id);
