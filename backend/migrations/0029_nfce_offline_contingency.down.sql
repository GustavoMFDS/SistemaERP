-- 0029_nfce_offline_contingency.down.sql

BEGIN;

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_contingency_metadata_check;

ALTER TABLE invoices
  DROP COLUMN IF EXISTS contingency_reason,
  DROP COLUMN IF EXISTS contingency_started_at;

COMMIT;
