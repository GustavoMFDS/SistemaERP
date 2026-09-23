-- 0019_sale_returns.up.sql

BEGIN;

-- Devolucoes/trocas have their own permission. Existing roles that were
-- allowed to cancel sales inherit it during the migration.
INSERT INTO permissions(id, code, description)
VALUES (gen_random_uuid(), 'sale:return', 'Registrar devolucoes e trocas')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT rp.role_id, target.id
FROM role_permissions rp
JOIN permissions source ON source.id=rp.permission_id AND source.code='sale:cancel'
JOIN permissions target ON target.code='sale:return'
ON CONFLICT DO NOTHING;

-- Composite keys are used by the returns module so direct SQL cannot connect
-- rows belonging to different tenants.
CREATE UNIQUE INDEX IF NOT EXISTS sales_tenant_id_id_unique
  ON sales(tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS sale_items_tenant_id_id_unique
  ON sale_items(tenant_id, id);

CREATE TABLE IF NOT EXISTS sale_returns (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  sale_id uuid NOT NULL REFERENCES sales(id) ON DELETE RESTRICT,
  kind text NOT NULL DEFAULT 'return',
  reason text NOT NULL,
  refund_due numeric(12,2) NOT NULL DEFAULT 0,
  replacement_sale_id uuid NULL REFERENCES sales(id) ON DELETE RESTRICT,
  created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT sale_returns_kind_check CHECK (kind IN ('return','exchange')),
  CONSTRAINT sale_returns_reason_nonempty CHECK (length(trim(reason)) >= 3),
  CONSTRAINT sale_returns_refund_due_nonnegative CHECK (refund_due >= 0),
  CONSTRAINT sale_returns_tenant_id_id_unique UNIQUE (tenant_id, id),
  CONSTRAINT sale_returns_sale_tenant_fk
    FOREIGN KEY (tenant_id, sale_id) REFERENCES sales(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT sale_returns_replacement_sale_tenant_fk
    FOREIGN KEY (tenant_id, replacement_sale_id) REFERENCES sales(tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS sale_returns_tenant_sale_created_idx
  ON sale_returns(tenant_id, sale_id, created_at DESC);

CREATE TABLE IF NOT EXISTS sale_return_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  return_id uuid NOT NULL REFERENCES sale_returns(id) ON DELETE RESTRICT,
  sale_item_id uuid NOT NULL REFERENCES sale_items(id) ON DELETE RESTRICT,
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
  qty numeric(14,3) NOT NULL,
  restock boolean NOT NULL DEFAULT true,
  refund_value numeric(12,2) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT sale_return_items_qty_positive CHECK (qty > 0),
  CONSTRAINT sale_return_items_refund_nonnegative CHECK (refund_value >= 0),
  CONSTRAINT sale_return_items_return_sale_item_unique UNIQUE (return_id, sale_item_id),
  CONSTRAINT sale_return_items_return_tenant_fk
    FOREIGN KEY (tenant_id, return_id) REFERENCES sale_returns(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT sale_return_items_sale_item_tenant_fk
    FOREIGN KEY (tenant_id, sale_item_id) REFERENCES sale_items(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT sale_return_items_product_tenant_fk
    FOREIGN KEY (tenant_id, product_id) REFERENCES products(tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS sale_return_items_tenant_sale_item_idx
  ON sale_return_items(tenant_id, sale_item_id, created_at DESC);

CREATE TABLE IF NOT EXISTS return_idempotency_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  operation text NOT NULL,
  idem_key text NOT NULL,
  request_hash text NOT NULL,
  return_id uuid NOT NULL,
  refund_due numeric(12,2) NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT return_idempotency_key_unique UNIQUE (tenant_id, operation, idem_key),
  CONSTRAINT return_idempotency_refund_due_nonnegative CHECK (refund_due >= 0),
  CONSTRAINT return_idempotency_return_tenant_fk
    FOREIGN KEY (tenant_id, return_id) REFERENCES sale_returns(tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS return_idempotency_tenant_created_idx
  ON return_idempotency_keys(tenant_id, operation, created_at DESC);

DROP TRIGGER IF EXISTS return_idempotency_keys_immutable ON return_idempotency_keys;
CREATE TRIGGER return_idempotency_keys_immutable
  BEFORE UPDATE ON return_idempotency_keys
  FOR EACH ROW EXECUTE FUNCTION prevent_idempotency_key_update();

COMMIT;
