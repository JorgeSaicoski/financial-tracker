-- payment_methods is the per-user, extendable registry replacing the old
-- fixed enum (BACK-17) — same shape as currencies, but scoped per user.
-- 'credit_card' and 'bank_transfer' are system entries: code branches on
-- them directly (installment validation, transfer legs), so the API
-- rejects creating/renaming/deleting them — lazily ensured per user on
-- first read/write rather than backfilled here, same absence-safe
-- pattern as user_settings/categories.
CREATE TABLE IF NOT EXISTS payment_methods (
    id         TEXT    PRIMARY KEY,
    user_id    TEXT    NOT NULL,
    name       TEXT    NOT NULL,
    created_at TEXT    NOT NULL
);

-- Unique per the ticket's own requirement: (user_id, lower(name)) — a
-- functional index, checked at insert time instead of relying on the
-- application layer's list-then-compare (which is race-prone under
-- concurrent requests). SQLite supports expression indexes since 3.9.
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_methods_user_name ON payment_methods (user_id, lower(name));

-- One-time backfill: every distinct payment_method value already used in
-- movements becomes an ordinary per-user row — this naturally preserves
-- any existing 'pix' movements as an ordinary per-user registry row, it
-- just stops being baked into the binary; no movement's stored
-- payment_method string changes. Grouped by lower(payment_method) so two
-- case-variant spellings for the same user collapse into one row instead
-- of violating the unique index above; MIN(payment_method) picks a
-- stable representative spelling. 'credit_card'/'bank_transfer' are
-- included here too (nothing to exclude — unlike categories' system
-- names, these are ordinary rows once seeded, just protected from
-- edit/delete at the API layer).
INSERT INTO payment_methods (id, user_id, name, created_at)
SELECT
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' ||
        substr(lower(hex(randomblob(2))), 2) || '-' ||
        substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
        lower(hex(randomblob(6))),
    user_id, name, strftime('%Y-%m-%dT%H:%M:%f000000Z', 'now')
FROM (
    SELECT user_id, MIN(payment_method) AS name
    FROM movements
    GROUP BY user_id, lower(payment_method)
);
