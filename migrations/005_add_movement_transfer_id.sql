-- transfer_id links the two movement rows (debit on the source account,
-- credit on the destination) that make up one account-to-account
-- transfer. Local-only, like account_id: both legs sync to ledger-service
-- independently and net to zero there.
ALTER TABLE movements ADD COLUMN transfer_id TEXT;

CREATE INDEX IF NOT EXISTS idx_movements_transfer
    ON movements (transfer_id) WHERE transfer_id IS NOT NULL;
