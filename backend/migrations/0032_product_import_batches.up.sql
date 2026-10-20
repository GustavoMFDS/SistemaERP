-- v32: atomic product CSV import receipts, scoped to one independent company.
BEGIN;
CREATE TABLE product_import_batches (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
  idem_key text NOT NULL CHECK (char_length(idem_key) BETWEEN 8 AND 128),
  request_hash text NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
  item_count integer NOT NULL CHECK (item_count BETWEEN 1 AND 500),
  created_by_user_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT product_import_batches_actor_tenant_fk
    FOREIGN KEY (created_by_user_id, tenant_id)
    REFERENCES user_tenants(user_id, tenant_id) ON DELETE RESTRICT,
  CONSTRAINT product_import_batches_tenant_key_unique UNIQUE (tenant_id, idem_key)
);
CREATE INDEX product_import_batches_tenant_created_idx
  ON product_import_batches (tenant_id, created_at DESC);
COMMIT;
