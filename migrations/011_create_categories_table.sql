-- categories is the per-user, extendable registry replacing the old
-- fixed enum (BACK-14) — same shape as currencies, but scoped per user,
-- with an avoidability_percent (0-100, how easy this kind of spend is
-- to skip) each category carries. 'transfer' and 'income' are system
-- categories: avoidability_percent stays NULL (they aren't spend at
-- all) and the API rejects creating/renaming/deleting them — lazily
-- ensured per user on first read/write rather than backfilled here,
-- same absence-safe pattern as user_settings.
CREATE TABLE IF NOT EXISTS categories (
    id                   TEXT    PRIMARY KEY,
    user_id              TEXT    NOT NULL,
    name                 TEXT    NOT NULL,
    avoidability_percent INTEGER,
    created_at           TEXT    NOT NULL
);

-- Unique per the ticket's own requirement: (user_id, lower(name)) — a
-- functional index, checked at insert time instead of relying on the
-- application layer's list-then-compare (which is race-prone under
-- concurrent requests). SQLite supports expression indexes since 3.9.
CREATE UNIQUE INDEX IF NOT EXISTS idx_categories_user_name ON categories (user_id, lower(name));

-- One-time backfill: every distinct non-system category value already
-- used in movements becomes an ordinary per-user row at a neutral 50%
-- default (the user edits afterward) — no movement's stored category
-- string changes. Grouped by lower(category) so two case-variant spellings
-- for the same user ("Food" and "food") collapse into one row instead of
-- violating the unique index above; MIN(category) picks a stable
-- representative spelling. System categories are excluded here; they're
-- lazily ensured with NULL avoidability instead (see above).
INSERT INTO categories (id, user_id, name, avoidability_percent, created_at)
SELECT
    lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' ||
        substr(lower(hex(randomblob(2))), 2) || '-' ||
        substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
        lower(hex(randomblob(6))),
    user_id, name, 50, strftime('%Y-%m-%dT%H:%M:%f000000Z', 'now')
FROM (
    SELECT user_id, MIN(category) AS name
    FROM movements
    WHERE lower(category) NOT IN ('transfer', 'income')
    GROUP BY user_id, lower(category)
);
