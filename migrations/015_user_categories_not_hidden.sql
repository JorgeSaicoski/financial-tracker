-- BACK-14 follow-up, part 3 (Jorge, 2026-07-28): "there is no hidden
-- category" — migration 014's user_hidden_categories was an invented
-- opt-out model that doesn't match what was actually asked for.
-- Removing a category from your own list isn't opting out of a
-- default-everyone-has-everything state; there is no such default.
-- "Has" is a plain positive membership fact: this table says exactly
-- who currently has which category, nothing more, nothing implied.
--
-- category_maintainers (edit rights) is untouched — contributor and
-- "has" stay two fully separate facts, per the same review comment.
CREATE TABLE IF NOT EXISTS user_categories (
    user_id     TEXT NOT NULL,
    category_id TEXT NOT NULL REFERENCES categories(id),
    PRIMARY KEY (user_id, category_id)
);

-- Backfill: today's "has" is exactly what CategoryRepository.ListForUser
-- already computed (contributor and not in user_hidden_categories) —
-- preserve that as the starting state of the new table before dropping
-- the old one out from under it.
INSERT INTO user_categories (user_id, category_id)
SELECT cm.user_id, cm.category_id
FROM category_maintainers cm
WHERE NOT EXISTS (
    SELECT 1 FROM user_hidden_categories uhc
    WHERE uhc.user_id = cm.user_id AND uhc.category_id = cm.category_id
);

DROP TABLE user_hidden_categories;
