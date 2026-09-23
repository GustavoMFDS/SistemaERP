-- 0014_cash_reconciliation.up.sql

BEGIN;

ALTER TABLE cash_sessions
  ADD COLUMN IF NOT EXISTS expected_cash numeric(12,2) NULL,
  ADD COLUMN IF NOT EXISTS closing_difference numeric(12,2) NULL;

COMMIT;
