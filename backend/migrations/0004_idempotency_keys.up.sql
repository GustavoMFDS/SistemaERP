-- 0004_idempotency_keys.up.sql

BEGIN;

-- Stores completed results for idempotent operations.
-- Used by PDV offline/sync to avoid duplicate writes on retries.
CREATE TABLE IF NOT EXISTS idempotency_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  operation text NOT NULL,
  idem_key text NOT NULL,
  sale_id uuid NOT NULL,
  total numeric(14,2) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, operation, idem_key)
);

CREATE INDEX IF NOT EXISTS idempotency_keys_tenant_op_created_idx
  ON idempotency_keys(tenant_id, operation, created_at DESC);

COMMIT;
