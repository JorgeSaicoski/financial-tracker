-- BACK-14 left movements.category, credit_card_purchases.category, and
-- recurring_rules.category as free-form strings resolved against the
-- categories registry by name at write time — flagged during that PR's
-- review as a lock-in concern: a plain string doesn't let a rename
-- propagate to historical rows, and doesn't give a future feature (a
-- shared/global category hub) anything stable to attach to. This
-- migration turns all three into a real category_id foreign key.
-- Postgres allows this in place — no table rebuild needed, unlike
-- SQLite's own version of this migration.
ALTER TABLE credit_card_purchases ADD COLUMN category_id TEXT REFERENCES categories(id);

UPDATE credit_card_purchases
SET category_id = c.id
FROM categories c
WHERE lower(c.name) = lower(credit_card_purchases.category)
  AND credit_card_purchases.category IS NOT NULL
  AND credit_card_purchases.category != '';

-- Guard against silently losing a category string that didn't match any
-- row in the categories registry (stale/renamed name) — without this,
-- the DROP COLUMN below would delete that data with no trace. Not a
-- DO $$ ... $$ block: this migration runner's splitStatements (db.go)
-- splits on every top-level ';', which would break a dollar-quoted
-- block apart into invalid fragments. A CHECK constraint on a
-- throwaway temp table stands in for an assertion instead — the INSERT
-- fails (aborting this migration's transaction) if any row is still
-- unresolved, and every line here stays a single, independently valid
-- statement.
CREATE TEMP TABLE assert_credit_card_purchases_category_backfilled (ok BOOLEAN NOT NULL CHECK (ok));
INSERT INTO assert_credit_card_purchases_category_backfilled (ok)
SELECT NOT EXISTS (
    SELECT 1 FROM credit_card_purchases WHERE category IS NOT NULL AND category != '' AND category_id IS NULL
);
DROP TABLE assert_credit_card_purchases_category_backfilled;

ALTER TABLE credit_card_purchases DROP COLUMN category;

-- recurring_rules.category still carried BACK-14's *old* fixed-enum
-- CHECK constraint (migration 013 only rebuilt movements/
-- credit_card_purchases, not this table) — meaning a free-form category
-- name outside that original list has been silently rejected by
-- POST /recurring-rules this whole time, a live bug this migration
-- fixes as a side effect of dropping the column entirely.
ALTER TABLE recurring_rules ADD COLUMN category_id TEXT REFERENCES categories(id);

UPDATE recurring_rules
SET category_id = c.id
FROM categories c
WHERE lower(c.name) = lower(recurring_rules.category)
  AND recurring_rules.category IS NOT NULL
  AND recurring_rules.category != '';

CREATE TEMP TABLE assert_recurring_rules_category_backfilled (ok BOOLEAN NOT NULL CHECK (ok));
INSERT INTO assert_recurring_rules_category_backfilled (ok)
SELECT NOT EXISTS (
    SELECT 1 FROM recurring_rules WHERE category IS NOT NULL AND category != '' AND category_id IS NULL
);
DROP TABLE assert_recurring_rules_category_backfilled;

ALTER TABLE recurring_rules DROP COLUMN category;

ALTER TABLE movements ADD COLUMN category_id TEXT REFERENCES categories(id);

UPDATE movements
SET category_id = c.id
FROM categories c
WHERE lower(c.name) = lower(movements.category)
  AND movements.category IS NOT NULL
  AND movements.category != '';

CREATE TEMP TABLE assert_movements_category_backfilled (ok BOOLEAN NOT NULL CHECK (ok));
INSERT INTO assert_movements_category_backfilled (ok)
SELECT NOT EXISTS (
    SELECT 1 FROM movements WHERE category IS NOT NULL AND category != '' AND category_id IS NULL
);
DROP TABLE assert_movements_category_backfilled;

ALTER TABLE movements DROP COLUMN category;

CREATE INDEX IF NOT EXISTS idx_movements_category ON movements (category_id) WHERE category_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_credit_card_purchases_category ON credit_card_purchases (category_id) WHERE category_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_recurring_rules_category ON recurring_rules (category_id) WHERE category_id IS NOT NULL;

-- --------------------------------------------------------------------
-- Part 2: categories become genuinely shared, not a private per-user
-- registry — Jorge's actual review comment on #27 ("I will create
-- restaurant category with 80% and offer it for whoever wants to get
-- it — if someone doesn't agree they can just create a new one; for now
-- even 2 identical categories will be 2 different ids, one per
-- creator"). Folded into this same migration rather than layered as a
-- new one on top of a design (per-user is_default, per-user ownership)
-- that's about to be wrong, since #39 hadn't merged yet when this was
-- written.
--
-- There's no "owner" column at all anymore, and no name-uniqueness
-- constraint — two different people can each have their own
-- "restaurant" category, and nothing here stops it (deduplicating that
-- is explicitly future work per Jorge, not this migration's job).
-- category_maintainers is the *only* place ownership/edit-rights are
-- tracked; migration 011's (user_id, lower(name)) unique index and the
-- user_id column it's built on are both removed below.
-- --------------------------------------------------------------------

DROP INDEX idx_categories_user_name;
ALTER TABLE categories DROP COLUMN user_id;

-- category_maintainers: who can rename a category or change its
-- avoidability_percent (see entities.Category.CanBeEditedBy /
-- CategoryRepository.IsContributor). A category's creator is always
-- inserted here in the same transaction as its creation — the three
-- system categories seeded below have zero rows here, which is also
-- what makes them uneditable by anyone (no separate "is system" flag
-- needed at the database level for that half of it).
CREATE TABLE IF NOT EXISTS category_maintainers (
    category_id TEXT NOT NULL REFERENCES categories(id),
    user_id     TEXT NOT NULL,
    PRIMARY KEY (category_id, user_id)
);

-- user_hidden_categories: BACK-14 follow-up's actual answer to "delete"
-- once a category isn't exclusively yours — "there is no real delete,
-- delete = removing from user categories list, the category is still
-- there because others can choose it." Presence marks that a user
-- opted this category out of their own future use — never touches the
-- category row itself or any other user's data. See
-- CategoryRepository.Hide / deleteCategoryUseCase.
CREATE TABLE IF NOT EXISTS user_hidden_categories (
    user_id     TEXT NOT NULL,
    category_id TEXT NOT NULL REFERENCES categories(id),
    PRIMARY KEY (user_id, category_id)
);

-- transfer/income/other become three fixed, global rows instead of
-- being lazily duplicated per user — seeded here with fixed ids rather
-- than generated at first use, specifically so
-- internal/cmd/migrate-sqlite's source and target databases always
-- agree on these three ids and its id-preserving copyCategories step
-- doesn't need to remap category_id references for them (it just skips
-- re-inserting rows the target's own migration run already seeded).
-- "other" carries a neutral 50% avoidability, same default a brand-new
-- user-created category gets; transfer/income stay NULL (not spend).
INSERT INTO categories (id, name, avoidability_percent, created_at) VALUES
    ('00000000-0000-0000-0000-000000000101', 'transfer', NULL, '2026-01-01T00:00:00Z'),
    ('00000000-0000-0000-0000-000000000102', 'income',   NULL, '2026-01-01T00:00:00Z'),
    ('00000000-0000-0000-0000-000000000103', 'other',    50,   '2026-01-01T00:00:00Z');

-- limits: operator-configurable numeric limits — a name/description/
-- value row per limit, so retuning one (e.g. raising
-- max_categories_per_user for a paid plan later) is a database update,
-- not a code change/redeploy. See createCategoryUseCase.
CREATE TABLE IF NOT EXISTS limits (
    name        TEXT PRIMARY KEY,
    description TEXT,
    value       INTEGER NOT NULL
);
INSERT INTO limits (name, description, value) VALUES
    ('max_categories_per_user', 'Maximum number of categories a single user may create/maintain', 10);

-- user_settings.default_category_id: a user's own fallback category for
-- the "reassign my existing movements" option on removing a category
-- from their list (see deleteCategoryUseCase). A personal preference
-- about a shared thing, not a property the category itself can carry —
-- NULL (never set) resolves to the global "other" row above.
ALTER TABLE user_settings ADD COLUMN default_category_id TEXT REFERENCES categories(id);
