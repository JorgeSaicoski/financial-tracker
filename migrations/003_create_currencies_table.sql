-- currencies is the user-extendable registry backing the frontend's
-- currency dropdown (GET /currencies). Movements store currency as plain
-- text, so the seed also backfills any code already present in the data.
CREATE TABLE IF NOT EXISTS currencies (
    code       TEXT PRIMARY KEY,
    created_at TEXT NOT NULL
);

INSERT OR IGNORE INTO currencies (code, created_at)
VALUES ('usd', strftime('%Y-%m-%dT%H:%M:%f000000Z', 'now')),
       ('brl', strftime('%Y-%m-%dT%H:%M:%f000000Z', 'now'));

INSERT OR IGNORE INTO currencies (code, created_at)
SELECT DISTINCT currency, strftime('%Y-%m-%dT%H:%M:%f000000Z', 'now')
FROM movements;
