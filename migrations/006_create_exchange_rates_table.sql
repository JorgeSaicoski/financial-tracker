-- exchange_rates are user-entered, per-user, historical conversion rates
-- against USD ("1 usd = 5 brl" -> units_per_usd = 5). Never overwritten in
-- place across dates: a new effective_from appends a row so a movement
-- converts at the rate that was true at its own timestamp. Posting the
-- same (user, currency, effective_from) again replaces that row (backfill
-- correction), handled by the repository's upsert. usd itself needs no
-- rows (implicitly 1). units_per_usd is TEXT, never a float column, so
-- the application layer's exact decimal math never loses precision.
CREATE TABLE IF NOT EXISTS exchange_rates (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL,
    currency       TEXT NOT NULL,
    units_per_usd  TEXT NOT NULL,
    effective_from TEXT NOT NULL,
    created_at     TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_exchange_rates_user_currency_effective
    ON exchange_rates (user_id, currency, effective_from);

-- Picking "the rate valid at time T" is a lookup for the greatest
-- effective_from <= T for (user, currency); DESC ordering serves that scan.
CREATE INDEX IF NOT EXISTS idx_exchange_rates_lookup
    ON exchange_rates (user_id, currency, effective_from DESC);
