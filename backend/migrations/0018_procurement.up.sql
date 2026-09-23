-- 0018_procurement.up.sql

BEGIN;

CREATE TABLE IF NOT EXISTS suppliers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  name text NOT NULL,
  document text NULL,
  email citext NULL,
  phone text NULL,
  contact_name text NULL,
  notes text NULL,
  active boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT suppliers_name_nonempty CHECK (length(trim(name)) > 0),
  CONSTRAINT suppliers_tenant_id_id_unique UNIQUE (tenant_id, id)
);

CREATE UNIQUE INDEX IF NOT EXISTS suppliers_tenant_document_unique
  ON suppliers(tenant_id, document)
  WHERE document IS NOT NULL;
CREATE INDEX IF NOT EXISTS suppliers_tenant_name_idx ON suppliers(tenant_id, name);

-- Required for composite tenant-safe foreign keys from procurement tables.
CREATE UNIQUE INDEX IF NOT EXISTS products_tenant_id_id_unique
  ON products(tenant_id, id);

CREATE TABLE IF NOT EXISTS purchases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  supplier_id uuid NOT NULL REFERENCES suppliers(id) ON DELETE RESTRICT,
  status text NOT NULL DEFAULT 'ordered',
  invoice_number text NULL,
  payment_due_date date NULL,
  total numeric(12,2) NOT NULL DEFAULT 0,
  notes text NULL,
  created_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  ordered_at timestamptz NOT NULL DEFAULT now(),
  received_at timestamptz NULL,
  cancelled_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT purchases_status_check CHECK (status IN ('ordered','partially_received','received','cancelled')),
  CONSTRAINT purchases_total_nonnegative CHECK (total >= 0),
  CONSTRAINT purchases_tenant_id_id_unique UNIQUE (tenant_id, id),
  CONSTRAINT purchases_supplier_tenant_fk
    FOREIGN KEY (tenant_id, supplier_id) REFERENCES suppliers(tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS purchases_tenant_status_created_idx
  ON purchases(tenant_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS purchases_tenant_supplier_created_idx
  ON purchases(tenant_id, supplier_id, created_at DESC);

CREATE TABLE IF NOT EXISTS purchase_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  purchase_id uuid NOT NULL REFERENCES purchases(id) ON DELETE CASCADE,
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
  qty_ordered numeric(14,3) NOT NULL,
  qty_received numeric(14,3) NOT NULL DEFAULT 0,
  unit_cost numeric(12,2) NOT NULL,
  line_total numeric(12,2) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT purchase_items_qty_ordered_positive CHECK (qty_ordered > 0),
  CONSTRAINT purchase_items_qty_received_valid CHECK (qty_received >= 0 AND qty_received <= qty_ordered),
  CONSTRAINT purchase_items_unit_cost_positive CHECK (unit_cost > 0),
  CONSTRAINT purchase_items_line_total_nonnegative CHECK (line_total >= 0),
  CONSTRAINT purchase_items_purchase_product_unique UNIQUE (purchase_id, product_id),
  CONSTRAINT purchase_items_tenant_id_id_unique UNIQUE (tenant_id, id),
  CONSTRAINT purchase_items_purchase_tenant_fk
    FOREIGN KEY (tenant_id, purchase_id) REFERENCES purchases(tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT purchase_items_product_tenant_fk
    FOREIGN KEY (tenant_id, product_id) REFERENCES products(tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS purchase_items_tenant_product_idx
  ON purchase_items(tenant_id, product_id);

CREATE TABLE IF NOT EXISTS purchase_receipts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  purchase_id uuid NOT NULL REFERENCES purchases(id) ON DELETE RESTRICT,
  received_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  notes text NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT purchase_receipts_tenant_id_id_unique UNIQUE (tenant_id, id),
  CONSTRAINT purchase_receipts_purchase_tenant_fk
    FOREIGN KEY (tenant_id, purchase_id) REFERENCES purchases(tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS purchase_receipts_tenant_purchase_idx
  ON purchase_receipts(tenant_id, purchase_id, received_at DESC);

CREATE TABLE IF NOT EXISTS purchase_receipt_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  receipt_id uuid NOT NULL REFERENCES purchase_receipts(id) ON DELETE CASCADE,
  purchase_item_id uuid NOT NULL REFERENCES purchase_items(id) ON DELETE RESTRICT,
  product_id uuid NOT NULL REFERENCES products(id) ON DELETE RESTRICT,
  qty numeric(14,3) NOT NULL,
  unit_cost numeric(12,2) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT purchase_receipt_items_qty_positive CHECK (qty > 0),
  CONSTRAINT purchase_receipt_items_unit_cost_positive CHECK (unit_cost > 0),
  CONSTRAINT purchase_receipt_items_receipt_tenant_fk
    FOREIGN KEY (tenant_id, receipt_id) REFERENCES purchase_receipts(tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT purchase_receipt_items_purchase_item_tenant_fk
    FOREIGN KEY (tenant_id, purchase_item_id) REFERENCES purchase_items(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT purchase_receipt_items_product_tenant_fk
    FOREIGN KEY (tenant_id, product_id) REFERENCES products(tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS purchase_receipt_items_tenant_product_idx
  ON purchase_receipt_items(tenant_id, product_id, created_at DESC);

ALTER TABLE accounts_payable
  ADD COLUMN IF NOT EXISTS supplier_id uuid NULL REFERENCES suppliers(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS purchase_id uuid NULL REFERENCES purchases(id) ON DELETE SET NULL;

ALTER TABLE accounts_payable
  DROP CONSTRAINT IF EXISTS accounts_payable_supplier_tenant_fk,
  ADD CONSTRAINT accounts_payable_supplier_tenant_fk
    FOREIGN KEY (tenant_id, supplier_id) REFERENCES suppliers(tenant_id, id) ON DELETE SET NULL (supplier_id),
  DROP CONSTRAINT IF EXISTS accounts_payable_purchase_tenant_fk,
  ADD CONSTRAINT accounts_payable_purchase_tenant_fk
    FOREIGN KEY (tenant_id, purchase_id) REFERENCES purchases(tenant_id, id) ON DELETE SET NULL (purchase_id);

CREATE UNIQUE INDEX IF NOT EXISTS accounts_payable_tenant_purchase_unique
  ON accounts_payable(tenant_id, purchase_id)
  WHERE purchase_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS procurement_idempotency_keys (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  operation text NOT NULL,
  idem_key text NOT NULL,
  request_hash text NOT NULL,
  resource_id uuid NOT NULL,
  result_status text NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT procurement_idempotency_operation_key_unique UNIQUE (tenant_id, operation, idem_key)
);

CREATE INDEX IF NOT EXISTS procurement_idempotency_tenant_created_idx
  ON procurement_idempotency_keys(tenant_id, operation, created_at DESC);

DROP TRIGGER IF EXISTS procurement_idempotency_keys_immutable ON procurement_idempotency_keys;
CREATE TRIGGER procurement_idempotency_keys_immutable
  BEFORE UPDATE ON procurement_idempotency_keys
  FOR EACH ROW EXECUTE FUNCTION prevent_idempotency_key_update();

COMMIT;
