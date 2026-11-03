-- 0003_ledger.down.sql
ALTER TABLE records
  DROP KEY idx_redemption,
  DROP COLUMN redemption_id;
DROP TABLE IF EXISTS redemptions;
