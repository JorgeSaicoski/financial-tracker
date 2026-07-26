-- cards are credit-card profiles (BACK-08): a closing day (statement
-- cut) and due day (when the installment actually hits the user's
-- money), plus optional credit_limit (the bank's real ceiling) and
-- monthly_budget (the user's own, independent spending goal — can be
-- lower than credit_limit). closing_day/due_day are "1"-"28" or "last",
-- same convention recurring rules use, validated in the application
-- layer (entities.ValidCardDay).
CREATE TABLE IF NOT EXISTS cards (
    id              TEXT        PRIMARY KEY,
    user_id         TEXT        NOT NULL,
    name            TEXT        NOT NULL,
    last_four       TEXT,
    closing_day     TEXT        NOT NULL,
    due_day         TEXT        NOT NULL,
    credit_limit    BIGINT,
    monthly_budget  BIGINT,
    currency        TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_cards_user ON cards (user_id, name);

-- card_id: which card a credit-card purchase/installment was made on
-- (nil keeps today's flat-offset date behavior, unchanged). Threaded
-- onto both movements (single, non-installment credit-card charges) and
-- credit_card_purchases (installment purchases) so either shape can
-- carry a card association.
ALTER TABLE movements ADD COLUMN card_id TEXT REFERENCES cards(id);
ALTER TABLE credit_card_purchases ADD COLUMN card_id TEXT REFERENCES cards(id);

-- card_payment_for_card_id: set only on a payment movement (an ordinary
-- expense on the paying account, category=transfer) that settles this
-- card's statement — reduces the card's next_due_total without ever
-- double-counting as a second expense, since transfer movements are
-- already excluded from cashflow category totals.
ALTER TABLE movements ADD COLUMN card_payment_for_card_id TEXT REFERENCES cards(id);

CREATE INDEX IF NOT EXISTS idx_movements_card ON movements (card_id) WHERE card_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_movements_card_payment ON movements (card_payment_for_card_id) WHERE card_payment_for_card_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_credit_card_purchases_card ON credit_card_purchases (card_id) WHERE card_id IS NOT NULL;
