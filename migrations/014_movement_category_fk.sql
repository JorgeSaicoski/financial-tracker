-- BACK-14 left movements.category and credit_card_purchases.category as
-- free-form strings resolved against the per-user categories registry by
-- name at write time (see application/usecases/categories.go's
-- resolveCategory) — flagged during that PR's review as a lock-in
-- concern: a plain string doesn't let a rename propagate to historical
-- rows, and doesn't give a future feature (e.g. a shared/global category
-- hub) anything stable to attach to. This migration turns both columns
-- into a real category_id foreign key.
--
-- Unlike migration 013 (which rebuilt movements/credit_card_purchases
-- wholesale because SQLite of that era couldn't drop a CHECK constraint
-- in place), this SQLite build supports ALTER TABLE ... DROP COLUMN
-- directly (verified against the exact driver this repo uses,
-- modernc.org/sqlite) — category isn't part of any index, unique
-- constraint, or CHECK here, so no rebuild is needed, just
-- add-backfill-drop.
--
-- Order matters: credit_card_purchases first, same reasoning 013 used —
-- movements holds a foreign key to it, so touching movements first would
-- leave that FK pointing at a table mid-change.
ALTER TABLE credit_card_purchases ADD COLUMN category_id TEXT REFERENCES categories(id);

UPDATE credit_card_purchases
SET category_id = (
    SELECT c.id FROM categories c
    WHERE c.user_id = credit_card_purchases.user_id AND lower(c.name) = lower(credit_card_purchases.category)
)
WHERE category IS NOT NULL AND category != '';

ALTER TABLE credit_card_purchases DROP COLUMN category;

ALTER TABLE movements ADD COLUMN category_id TEXT REFERENCES categories(id);

-- Migration 011 already guarantees a matching per-user category row for
-- every distinct historical category string (its own backfill's job),
-- so this lookup should never miss for a non-empty category.
UPDATE movements
SET category_id = (
    SELECT c.id FROM categories c
    WHERE c.user_id = movements.user_id AND lower(c.name) = lower(movements.category)
)
WHERE category IS NOT NULL AND category != '';

ALTER TABLE movements DROP COLUMN category;

CREATE INDEX IF NOT EXISTS idx_movements_category ON movements (category_id) WHERE category_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_credit_card_purchases_category ON credit_card_purchases (category_id) WHERE category_id IS NOT NULL;

-- is_default (BACK-14 follow-up): exactly one category per user can be
-- flagged as the fallback target movements/purchases get reassigned to
-- when their own category is deleted (see deleteCategoryUseCase) —
-- lazily seeded as "other" the same absence-safe way transfer/income
-- already are (see ensureDefaultCategory), not backfilled here.
ALTER TABLE categories ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1));
CREATE UNIQUE INDEX IF NOT EXISTS idx_categories_one_default_per_user ON categories (user_id) WHERE is_default = 1;
