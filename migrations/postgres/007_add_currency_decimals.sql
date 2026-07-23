-- decimals records how many minor-unit places a currency uses (2 for
-- usd/brl, 8 for btc, ...) so exchange-rate conversions can scale
-- smallest-unit amounts correctly instead of assuming 2 everywhere.
-- Existing rows default to 2 (correct for the seeded usd/brl and any
-- fiat code backfilled from movements so far); a currency that actually
-- needs a different value can be re-registered via a future admin path.
ALTER TABLE currencies ADD COLUMN decimals INTEGER NOT NULL DEFAULT 2;
