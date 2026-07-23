-- Mirrors ledger-service's own migrations/001_create_transactions_table.sql
-- — see deploy/ledger-postgres/init-db.sh for why this is a local copy
-- instead of a fetch/mount from a sibling checkout.

-- Enable pgcrypto for gen_random_uuid (may require superuser). If not available, the app can generate UUIDs.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS transactions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id TEXT NOT NULL,
  amount BIGINT NOT NULL,
  currency TEXT NOT NULL,
  timestamp TIMESTAMPTZ NOT NULL DEFAULT now()
);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'amount_check') THEN
        ALTER TABLE transactions ADD CONSTRAINT amount_check CHECK (amount <> 0);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_transactions_user_currency_time
  ON transactions(user_id, currency, timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_transactions_user_time
  ON transactions(user_id, timestamp DESC);
