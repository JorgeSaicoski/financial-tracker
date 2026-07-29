-- BACK-14 turned category into a free-form, per-user-registry-resolved
-- string (see migrations/011_create_categories_table.sql) instead of a
-- fixed enum — but 001_create_movements_table.sql's and
-- 002_create_credit_card_purchases_table.sql's CHECK constraints still
-- only allowed the old fixed list, so any category outside it (including
-- brand-new implicitly-registered ones) was silently rejected by the
-- database regardless of what the application layer allowed.
-- Postgres auto-names an inline column CHECK "<table>_<column>_check".
ALTER TABLE movements DROP CONSTRAINT IF EXISTS movements_category_check;
ALTER TABLE credit_card_purchases DROP CONSTRAINT IF EXISTS credit_card_purchases_category_check;
