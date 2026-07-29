-- Mirrors migrations/008_create_recurring_rules_table.sql — see its
-- comment for the day_of_month/rationale. accounts already exists by
-- this point (migrations/postgres/004), so the FK can be declared
-- directly here, unlike credit_card_purchases' historical ordering
-- constraint (see migrations/postgres/001's comment).
CREATE TABLE IF NOT EXISTS recurring_rules (
    id                TEXT        PRIMARY KEY,
    user_id           TEXT        NOT NULL,
    amount            BIGINT      NOT NULL,
    currency          TEXT        NOT NULL,
    description       TEXT,
    category          TEXT        NOT NULL DEFAULT 'other' CHECK (category IN (
        'food','transport','housing','utilities','health','entertainment',
        'shopping','education','income','transfer','other')),
    payment_method    TEXT        NOT NULL DEFAULT 'other' CHECK (payment_method IN (
        'cash','debit_card','credit_card','pix','bank_transfer','other')),
    account_id        TEXT        REFERENCES accounts(id),
    day_of_month      TEXT        NOT NULL,
    starts_at         TIMESTAMPTZ NOT NULL,
    ends_at           TIMESTAMPTZ,
    active            BOOLEAN     NOT NULL DEFAULT TRUE,
    last_generated_at TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recurring_rules_user ON recurring_rules (user_id);
CREATE INDEX IF NOT EXISTS idx_recurring_rules_active ON recurring_rules (active) WHERE active = TRUE;

ALTER TABLE movements ADD COLUMN recurring_rule_id TEXT REFERENCES recurring_rules(id);

CREATE INDEX IF NOT EXISTS idx_movements_recurring_rule
    ON movements (recurring_rule_id) WHERE recurring_rule_id IS NOT NULL;
