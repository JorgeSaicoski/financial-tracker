-- BACK-14 left movements.category and credit_card_purchases.category as
-- free-form strings resolved against the per-user categories registry by
-- name at write time (see application/usecases/categories.go's
-- resolveCategory) — flagged during that PR's review as a lock-in
-- concern: a plain string doesn't let a rename propagate to historical
-- rows, and doesn't give a future feature (e.g. a shared/global category
-- hub) anything stable to attach to. This migration turns both columns
-- into a real category_id foreign key. Postgres allows this in place —
-- no table rebuild needed, unlike SQLite's own version of this
-- migration.
ALTER TABLE credit_card_purchases ADD COLUMN category_id TEXT REFERENCES categories(id);

UPDATE credit_card_purchases
SET category_id = c.id
FROM categories c
WHERE c.user_id = credit_card_purchases.user_id
  AND lower(c.name) = lower(credit_card_purchases.category)
  AND credit_card_purchases.category IS NOT NULL
  AND credit_card_purchases.category != '';

ALTER TABLE credit_card_purchases DROP COLUMN category;

ALTER TABLE movements ADD COLUMN category_id TEXT REFERENCES categories(id);

-- Migration 011 already guarantees a matching per-user category row for
-- every distinct historical category string (its own backfill's job),
-- so this lookup should never miss for a non-empty category.
UPDATE movements
SET category_id = c.id
FROM categories c
WHERE c.user_id = movements.user_id
  AND lower(c.name) = lower(movements.category)
  AND movements.category IS NOT NULL
  AND movements.category != '';

ALTER TABLE movements DROP COLUMN category;

CREATE INDEX IF NOT EXISTS idx_movements_category ON movements (category_id) WHERE category_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_credit_card_purchases_category ON credit_card_purchases (category_id) WHERE category_id IS NOT NULL;

-- is_default (BACK-14 follow-up): exactly one category per user can be
-- flagged as the fallback target movements/purchases get reassigned to
-- when their own category is deleted (see deleteCategoryUseCase) —
-- lazily seeded as "other" the same absence-safe way transfer/income
-- already are (see ensureDefaultCategory), not backfilled here.
ALTER TABLE categories ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT false;
CREATE UNIQUE INDEX IF NOT EXISTS idx_categories_one_default_per_user ON categories (user_id) WHERE is_default;
