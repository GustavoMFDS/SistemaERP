-- 0022_payment_reconciliation_adjustments.down.sql

BEGIN;

DROP TRIGGER IF EXISTS payment_reconciliation_adjustments_immutable ON payment_reconciliation_adjustments;
DROP TRIGGER IF EXISTS payment_reconciliations_immutable ON payment_reconciliations;
DROP FUNCTION IF EXISTS prevent_payment_reconciliation_history_mutation();
DROP TABLE IF EXISTS payment_reconciliation_adjustments;

COMMIT;
