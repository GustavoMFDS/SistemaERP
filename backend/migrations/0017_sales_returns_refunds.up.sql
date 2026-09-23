-- 0017_sales_returns_refunds.up.sql

BEGIN;

CREATE TABLE IF NOT EXISTS sale_returns (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  sale_id uuid NOT NULL REFERENCES sales(id) ON DELETE RESTRICT,
  reason text NOT NULL,
  total_amount numeric(12,2) NOT NULL,
  recovered_cost numeric(12,2) NOT NULL DEFAULT 0,
  created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sale_returns_reason_nonempty CHECK (length(trim(reason)) >= 3),
  CONSTRAINT sale_returns_total_nonnegative CHECK (total_amount >= 0),
  CONSTRAINT sale_returns_recovered_cost_nonnegative CHECK (recovered_cost >= 0)
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
  amount numeric(12,2) NOT NULL,
  recovered_cost numeric(12,2) NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sale_return_items_qty_positive CHECK (qty > 0),
  CONSTRAINT sale_return_items_amount_nonnegative CHECK (amount >= 0),
  CONSTRAINT sale_return_items_recovered_cost_nonnegative CHECK (recovered_cost >= 0),
  CONSTRAINT sale_return_items_unique_per_return UNIQUE (return_id, sale_item_id)
);

CREATE INDEX IF NOT EXISTS sale_return_items_tenant_sale_item_idx
  ON sale_return_items(tenant_id, sale_item_id, created_at DESC);
CREATE INDEX IF NOT EXISTS sale_return_items_tenant_product_idx
  ON sale_return_items(tenant_id, product_id, created_at DESC);

CREATE TABLE IF NOT EXISTS sale_refunds (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  return_id uuid NOT NULL REFERENCES sale_returns(id) ON DELETE RESTRICT,
  sale_id uuid NOT NULL REFERENCES sales(id) ON DELETE RESTRICT,
  method text NOT NULL,
  amount numeric(12,2) NOT NULL,
  external_reference text NULL,
  notes text NULL,
  created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sale_refunds_method_check CHECK (method IN ('cash','pix','debit','credit','transfer','voucher','store_credit')),
  CONSTRAINT sale_refunds_amount_positive CHECK (amount > 0)
);

CREATE INDEX IF NOT EXISTS sale_refunds_tenant_return_created_idx
  ON sale_refunds(tenant_id, return_id, created_at DESC);
CREATE INDEX IF NOT EXISTS sale_refunds_tenant_sale_created_idx
  ON sale_refunds(tenant_id, sale_id, created_at DESC);

CREATE TABLE IF NOT EXISTS sales_return_idempotency_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  operation text NOT NULL,
  idem_key text NOT NULL,
  request_hash text NOT NULL,
  resource_id uuid NOT NULL,
  result_status text NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT sales_return_idempotency_operation_key_unique UNIQUE (tenant_id, operation, idem_key)
);

CREATE INDEX IF NOT EXISTS sales_return_idempotency_tenant_created_idx
  ON sales_return_idempotency_keys(tenant_id, operation, created_at DESC);

DROP TRIGGER IF EXISTS sales_return_idempotency_keys_immutable ON sales_return_idempotency_keys;
CREATE TRIGGER sales_return_idempotency_keys_immutable
  BEFORE UPDATE ON sales_return_idempotency_keys
  FOR EACH ROW EXECUTE FUNCTION prevent_idempotency_key_update();

COMMIT;
