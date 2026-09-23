-- 0015_cash_movements_and_reconciliation.up.sql
BEGIN;

CREATE TABLE IF NOT EXISTS cash_movements (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  cash_session_id uuid NOT NULL REFERENCES cash_sessions(id) ON DELETE RESTRICT,
  movement_type text NOT NULL,
  amount numeric(12,2) NOT NULL,
  notes text NULL,
  created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT cash_movements_type_check CHECK (movement_type IN ('supply','withdrawal')),
  CONSTRAINT cash_movements_amount_positive CHECK (amount > 0)
);

CREATE INDEX IF NOT EXISTS cash_movements_tenant_session_created_idx
  ON cash_movements(tenant_id, cash_session_id, created_at);

CREATE TABLE IF NOT EXISTS cash_session_reconciliations (
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  cash_session_id uuid NOT NULL REFERENCES cash_sessions(id) ON DELETE CASCADE,
  method text NOT NULL,
  expected_amount numeric(12,2) NOT NULL,
  declared_amount numeric(12,2) NOT NULL,
  difference_amount numeric(12,2) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (cash_session_id, method),
  CONSTRAINT cash_reconciliation_method_check CHECK (
    method IN ('cash','pix','debit','credit','transfer','voucher')
  )
);

INSERT INTO permissions (id, code, description)
VALUES (gen_random_uuid(), 'cash:move', 'Registrar sangria e suprimento de caixa')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code='cash:move'
WHERE r.name IN ('admin','manager')
ON CONFLICT DO NOTHING;

COMMIT;
