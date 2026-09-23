-- 0020_payments_reconciliation_and_refunds.up.sql

BEGIN;

INSERT INTO permissions(id, code, description)
VALUES (gen_random_uuid(), 'finance:reconcile', 'Conciliar pagamentos e liquidar reembolsos')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code='finance:reconcile'
WHERE r.name IN ('admin','manager')
ON CONFLICT DO NOTHING;

ALTER TABLE payments
  ADD COLUMN IF NOT EXISTS provider text NULL,
  ADD COLUMN IF NOT EXISTS transaction_ref text NULL,
  ADD COLUMN IF NOT EXISTS authorization_code text NULL,
  ADD COLUMN IF NOT EXISTS installments integer NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS reconciliation_status text NOT NULL DEFAULT 'pending',
  ADD COLUMN IF NOT EXISTS reconciled_amount numeric(12,2) NULL,
  ADD COLUMN IF NOT EXISTS reconciled_fee numeric(12,2) NULL,
  ADD COLUMN IF NOT EXISTS reconciled_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS reconciled_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS reconciliation_notes text NULL;

UPDATE payments
SET reconciliation_status='not_applicable'
WHERE method='cash' AND reconciliation_status='pending';

ALTER TABLE payments
  DROP CONSTRAINT IF EXISTS payments_installments_positive,
  ADD CONSTRAINT payments_installments_positive CHECK (installments > 0 AND installments <= 60),
  DROP CONSTRAINT IF EXISTS payments_reconciliation_status_check,
  ADD CONSTRAINT payments_reconciliation_status_check
    CHECK (reconciliation_status IN ('pending','reconciled','divergent','not_applicable')),
  DROP CONSTRAINT IF EXISTS payments_reconciled_amount_nonnegative,
  ADD CONSTRAINT payments_reconciled_amount_nonnegative CHECK (reconciled_amount IS NULL OR reconciled_amount >= 0),
  DROP CONSTRAINT IF EXISTS payments_reconciled_fee_nonnegative,
  ADD CONSTRAINT payments_reconciled_fee_nonnegative CHECK (reconciled_fee IS NULL OR reconciled_fee >= 0);

CREATE UNIQUE INDEX IF NOT EXISTS payments_tenant_id_id_unique
  ON payments(tenant_id, id);
CREATE INDEX IF NOT EXISTS payments_tenant_reconciliation_idx
  ON payments(tenant_id, reconciliation_status, method, created_at DESC);

CREATE TABLE IF NOT EXISTS payment_reconciliations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  payment_id uuid NOT NULL REFERENCES payments(id) ON DELETE RESTRICT,
  expected_amount numeric(12,2) NOT NULL,
  received_amount numeric(12,2) NOT NULL,
  fee_amount numeric(12,2) NOT NULL DEFAULT 0,
  net_amount numeric(12,2) NOT NULL,
  difference_amount numeric(12,2) NOT NULL,
  status text NOT NULL,
  provider text NULL,
  external_ref text NULL,
  notes text NULL,
  created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT payment_reconciliation_amounts_nonnegative CHECK (
    expected_amount >= 0 AND received_amount >= 0 AND fee_amount >= 0 AND net_amount >= 0
  ),
  CONSTRAINT payment_reconciliation_status_check CHECK (status IN ('reconciled','divergent')),
  CONSTRAINT payment_reconciliation_payment_tenant_fk
    FOREIGN KEY (tenant_id, payment_id) REFERENCES payments(tenant_id, id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX IF NOT EXISTS payment_reconciliations_tenant_payment_unique
  ON payment_reconciliations(tenant_id, payment_id);
CREATE INDEX IF NOT EXISTS payment_reconciliations_tenant_payment_created_idx
  ON payment_reconciliations(tenant_id, payment_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS cash_sessions_tenant_id_id_unique
  ON cash_sessions(tenant_id, id);

CREATE TABLE IF NOT EXISTS return_refunds (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  return_id uuid NOT NULL,
  sale_id uuid NOT NULL,
  method text NOT NULL,
  amount numeric(12,2) NOT NULL,
  provider text NULL,
  external_ref text NULL,
  cash_session_id uuid NULL,
  notes text NULL,
  created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT return_refunds_method_check CHECK (method IN ('cash','pix','debit','credit','transfer','voucher')),
  CONSTRAINT return_refunds_amount_positive CHECK (amount > 0),
  CONSTRAINT return_refunds_return_tenant_fk
    FOREIGN KEY (tenant_id, return_id) REFERENCES sale_returns(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT return_refunds_sale_tenant_fk
    FOREIGN KEY (tenant_id, sale_id) REFERENCES sales(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT return_refunds_cash_session_tenant_fk
    FOREIGN KEY (tenant_id, cash_session_id) REFERENCES cash_sessions(tenant_id, id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS return_refunds_tenant_return_created_idx
  ON return_refunds(tenant_id, return_id, created_at DESC);

CREATE TABLE IF NOT EXISTS finance_idempotency_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  operation text NOT NULL,
  idem_key text NOT NULL,
  request_hash text NOT NULL,
  resource_id uuid NOT NULL,
  result_status text NULL,
  result_amount numeric(12,2) NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT finance_idempotency_key_unique UNIQUE (tenant_id, operation, idem_key),
  CONSTRAINT finance_idempotency_result_amount_nonnegative CHECK (
    result_amount IS NULL OR result_amount >= 0
  )
);
CREATE INDEX IF NOT EXISTS finance_idempotency_tenant_created_idx
  ON finance_idempotency_keys(tenant_id, operation, created_at DESC);

DROP TRIGGER IF EXISTS finance_idempotency_keys_immutable ON finance_idempotency_keys;
CREATE TRIGGER finance_idempotency_keys_immutable
  BEFORE UPDATE ON finance_idempotency_keys
  FOR EACH ROW EXECUTE FUNCTION prevent_idempotency_key_update();

COMMIT;
