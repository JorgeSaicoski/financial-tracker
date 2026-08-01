-- avoidability_override_percent (BACK-14) is a movement's own ad-hoc
-- avoidability, for a genuine one-off spend that doesn't deserve its own
-- category (went karting once — 100% avoidable, never happening again).
-- When set it wins over the movement's category's avoidability_percent;
-- see application/usecases' effective-avoidability resolution helper.
ALTER TABLE movements ADD COLUMN avoidability_override_percent INTEGER;
