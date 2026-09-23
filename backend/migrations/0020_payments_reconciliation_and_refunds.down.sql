-- 0020_payments_reconciliation_and_refunds.down.sql

BEGIN;

DROP TABLE IF EXISTS finance_idempotency_keys;
DROP TABLE IF EXISTS return_refunds;
DROP TABLE IF EXISTS payment_reconciliations;

DROP INDEX IF EXISTS cash_sessions_tenant_id_id_unique;
DROP INDEX IF EXISTS payments_tenant_provider_transaction_ref_unique;
DROP INDEX IF EXISTS payments_tenant_reconciliation_idx;
DROP INDEX IF EXISTS payments_tenant_id_id_unique;

ALTER TABLE payments
  DROP CONSTRAINT IF EXISTS payments_reconciled_fee_nonnegative,
  DROP CONSTRAINT IF EXISTS payments_reconciled_amount_nonnegative,
  DROP CONSTRAINT IF EXISTS payments_reconciliation_status_check,
  DROP CONSTRAINT IF EXISTS payments_installments_positive,
  DROP COLUMN IF EXISTS reconciliation_notes,
  DROP COLUMN IF EXISTS reconciled_by_user_id,
  DROP COLUMN IF EXISTS reconciled_at,
  DROP COLUMN IF EXISTS reconciled_fee,
  DROP COLUMN IF EXISTS reconciled_amount,
  DROP COLUMN IF EXISTS reconciliation_status,
  DROP COLUMN IF EXISTS installments,
  DROP COLUMN IF EXISTS authorization_code,
  DROP COLUMN IF EXISTS transaction_ref,
  DROP COLUMN IF EXISTS provider;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code='finance:reconcile');
DELETE FROM permissions WHERE code='finance:reconcile';

COMMIT;
