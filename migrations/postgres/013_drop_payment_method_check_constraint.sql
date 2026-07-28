-- BACK-17 turned payment_method into a free-form, per-user-registry-
-- resolved string (see migrations/postgres/012_create_payment_methods_table.sql)
-- instead of a fixed enum — but 001_create_movements_table.sql's CHECK
-- constraint still only allowed the old fixed list ('pix' included), so
-- any payment method outside it (including brand-new implicitly-
-- registered ones) was silently rejected by the database regardless of
-- what the application layer allowed.
-- Postgres auto-names an inline column CHECK "<table>_<column>_check".
ALTER TABLE movements DROP CONSTRAINT IF EXISTS movements_payment_method_check;
