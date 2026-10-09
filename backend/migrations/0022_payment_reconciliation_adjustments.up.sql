-- 0022_payment_reconciliation_adjustments.up.sql

BEGIN;

CREATE TABLE IF NOT EXISTS payment_reconciliation_adjustments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  payment_id uuid NOT NULL,
  previous_received_amount numeric(12,2) NOT NULL,
  previous_fee_amount numeric(12,2) NOT NULL DEFAULT 0,
  new_received_amount numeric(12,2) NOT NULL,
  new_fee_amount numeric(12,2) NOT NULL DEFAULT 0,
  difference_amount numeric(12,2) NOT NULL,
  status text NOT NULL,
  notes text NULL,
  created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT payment_reconciliation_adjustment_amounts_check CHECK (
    previous_received_amount >= 0
    AND previous_fee_amount >= 0
    AND previous_fee_amount <= previous_received_amount
    AND new_received_amount >= 0
    AND new_fee_amount >= 0
    AND new_fee_amount <= new_received_amount
  ),
  CONSTRAINT payment_reconciliation_adjustment_status_check
    CHECK (status IN ('reconciled','divergent')),
  CONSTRAINT payment_reconciliation_adjustment_payment_tenant_fk
    FOREIGN KEY (tenant_id, payment_id) REFERENCES payments(tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS payment_reconciliation_adjustments_tenant_payment_created_idx
  ON payment_reconciliation_adjustments(tenant_id, payment_id, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_payment_reconciliation_history_mutation()
RETURNS trigger AS $history$
BEGIN
  RAISE EXCEPTION 'payment reconciliation history is immutable';
END;
$history$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS payment_reconciliations_immutable ON payment_reconciliations;
CREATE TRIGGER payment_reconciliations_immutable
  BEFORE UPDATE OR DELETE ON payment_reconciliations
  FOR EACH ROW EXECUTE FUNCTION prevent_payment_reconciliation_history_mutation();

DROP TRIGGER IF EXISTS payment_reconciliation_adjustments_immutable ON payment_reconciliation_adjustments;
CREATE TRIGGER payment_reconciliation_adjustments_immutable
  BEFORE UPDATE OR DELETE ON payment_reconciliation_adjustments
  FOR EACH ROW EXECUTE FUNCTION prevent_payment_reconciliation_history_mutation();

COMMIT;
