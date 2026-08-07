-- plans is BACK-10's monthly-figure goal, either a pure simulation
-- (plan_type='stress_test', never posts a movement) or funded by real
-- movements toward a real target (plan_type='savings'). target_amount
-- and account_id are set only for savings plans — a hypothetical cost
-- has no real target or funding account, enforced in the application
-- layer, not here.
CREATE TABLE IF NOT EXISTS plans (
    id                     TEXT        PRIMARY KEY,
    user_id                TEXT        NOT NULL,
    name                   TEXT        NOT NULL,
    plan_type              TEXT        NOT NULL CHECK (plan_type IN ('stress_test', 'savings')),
    target_amount          BIGINT,
    currency               TEXT        NOT NULL,
    monthly_target_amount  BIGINT      NOT NULL,
    account_id             TEXT        REFERENCES accounts(id),
    start_date             TIMESTAMPTZ NOT NULL,
    end_date               TIMESTAMPTZ,
    status                 TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'completed', 'abandoned')),
    created_at             TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_plans_user ON plans (user_id);

-- plan_id links a movement to the savings plan it funds — local-only,
-- like account_id/transfer_id: ledger-service never learns about plans.
-- Never set on a stress-test plan's movements, since one never has any.
ALTER TABLE movements ADD COLUMN plan_id TEXT REFERENCES plans(id);

CREATE INDEX IF NOT EXISTS idx_movements_plan
    ON movements (plan_id) WHERE plan_id IS NOT NULL;
