-- accounts are the places money actually sits (bank accounts, crypto
-- wallets, investment accounts). Movements optionally point at one so we
-- can compute each account's tracked balance.
CREATE TABLE IF NOT EXISTS accounts (
    id         TEXT        PRIMARY KEY,
    user_id    TEXT        NOT NULL,
    name       TEXT        NOT NULL,
    type       TEXT        NOT NULL DEFAULT 'other' CHECK (type IN (
        'bank','investment','crypto','cash','other')),
    currency   TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_accounts_user ON accounts (user_id, name);

-- account_snapshots are user-reported real balances ("my broker says I
-- have X today"). Comparing consecutive snapshots against the movements
-- recorded between them yields the account's return (interest/yield) —
-- money that appeared without a movement explaining it.
CREATE TABLE IF NOT EXISTS account_snapshots (
    id         TEXT        PRIMARY KEY,
    account_id TEXT        NOT NULL REFERENCES accounts(id),
    balance    BIGINT      NOT NULL,
    timestamp  TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_snapshots_account_time
    ON account_snapshots (account_id, timestamp DESC);

ALTER TABLE movements ADD COLUMN account_id TEXT REFERENCES accounts(id);

CREATE INDEX IF NOT EXISTS idx_movements_account
    ON movements (account_id) WHERE account_id IS NOT NULL;
