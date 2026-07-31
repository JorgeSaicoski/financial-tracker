-- subscriptions (BACK-19) tracks the paid annual "durable cloud storage"
-- tier: one current row per user, upserted by the payment provider's
-- webhook. This is the provider's view of billing state, not the
-- entitlement itself — user_settings.cloud_storage_entitled (BACK-13) is
-- what the rest of the app actually reads; this table only exists to
-- drive that flag and to answer "what does my subscription look like"
-- for GET /settings. status is "active" | "past_due" | "canceled".
-- current_period_end is when the current paid period (or, for
-- "past_due", the last successfully paid period) ends — the grace-period
-- sweep (internal/application/billing) uses it to decide when a late
-- payment should actually cost the user their entitlement.
CREATE TABLE IF NOT EXISTS subscriptions (
    user_id                    TEXT        PRIMARY KEY,
    provider                   TEXT        NOT NULL,
    provider_subscription_id   TEXT        NOT NULL,
    status                     TEXT        NOT NULL CHECK (status IN ('active', 'past_due', 'canceled')),
    current_period_end         TIMESTAMPTZ NOT NULL,
    created_at                 TIMESTAMPTZ NOT NULL,
    updated_at                 TIMESTAMPTZ NOT NULL,
    UNIQUE (provider, provider_subscription_id)
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_status_period_end
    ON subscriptions (status, current_period_end);
