-- currencies is the user-extendable registry backing the frontend's
-- currency dropdown (GET /currencies). Movements store currency as plain
-- text, so the seed also backfills any code already present in the data.
CREATE TABLE IF NOT EXISTS currencies (
    code       TEXT        PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL
);

INSERT INTO currencies (code, created_at)
VALUES ('usd', now()),
       ('brl', now())
ON CONFLICT (code) DO NOTHING;

INSERT INTO currencies (code, created_at)
SELECT DISTINCT currency, now()
FROM movements
ON CONFLICT (code) DO NOTHING;
