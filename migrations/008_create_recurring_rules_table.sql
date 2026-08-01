-- recurring_rules define fixed monthly movements (rent, salary,
-- subscriptions) that shouldn't need manual re-entry every month. A
-- background pass (see internal/application/recurring) generates one
-- ordinary movement per scheduled date since last_generated_at, up to
-- now. day_of_month is '1'..'28' or 'last' — never 29-31, so a fixed day
-- never silently drifts across months of different lengths; enforced in
-- the application layer (see entities.ValidDayOfMonth), not a CHECK,
-- since "last" isn't representable as a simple numeric range.
CREATE TABLE IF NOT EXISTS recurring_rules (
    id                TEXT    PRIMARY KEY,
    user_id           TEXT    NOT NULL,
    amount            INTEGER NOT NULL,
    currency          TEXT    NOT NULL,
    description       TEXT,
    category          TEXT    NOT NULL DEFAULT 'other' CHECK (category IN (
        'food','transport','housing','utilities','health','entertainment',
        'shopping','education','income','transfer','other')),
    payment_method    TEXT    NOT NULL DEFAULT 'other' CHECK (payment_method IN (
        'cash','debit_card','credit_card','pix','bank_transfer','other')),
    account_id        TEXT    REFERENCES accounts(id),
    day_of_month      TEXT    NOT NULL,
    starts_at         TEXT    NOT NULL,
    ends_at           TEXT,
    active            INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    last_generated_at TEXT,
    created_at        TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recurring_rules_user ON recurring_rules (user_id);
CREATE INDEX IF NOT EXISTS idx_recurring_rules_active ON recurring_rules (active) WHERE active = 1;

-- recurring_rule_id links a movement the generator created back to its
-- rule, purely for provenance/UI display — the movement is otherwise
-- ordinary (normal validation, sync_status=pending, cancellable/editable
-- like any other).
ALTER TABLE movements ADD COLUMN recurring_rule_id TEXT REFERENCES recurring_rules(id);

CREATE INDEX IF NOT EXISTS idx_movements_recurring_rule
    ON movements (recurring_rule_id) WHERE recurring_rule_id IS NOT NULL;
