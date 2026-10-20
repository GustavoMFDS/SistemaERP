-- 0030_opening_stock_batches.up.sql
BEGIN;
CREATE TABLE IF NOT EXISTS opening_stock_batches (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
  idem_key text NOT NULL,
  request_hash text NOT NULL,
  item_count integer NOT NULL CHECK (item_count BETWEEN 1 AND 100),
  created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT opening_stock_batches_key_len CHECK (char_length(idem_key) BETWEEN 1 AND 128),
  CONSTRAINT opening_stock_batches_hash_len CHECK (char_length(request_hash) = 64),
  CONSTRAINT opening_stock_batches_unique_key UNIQUE (tenant_id, idem_key)
);
CREATE INDEX IF NOT EXISTS opening_stock_batches_tenant_created
  ON opening_stock_batches(tenant_id, created_at DESC);
COMMIT;
