-- 0014_cash_reconciliation.down.sql

BEGIN;

ALTER TABLE cash_sessions
  DROP COLUMN IF EXISTS closing_difference,
  DROP COLUMN IF EXISTS expected_cash;

COMMIT;
