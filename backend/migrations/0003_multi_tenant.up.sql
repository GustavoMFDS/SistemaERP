-- 0003_multi_tenant.up.sql

BEGIN;

-- Users <-> Tenants (companies)
CREATE TABLE IF NOT EXISTS user_tenants (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, tenant_id)
);
CREATE INDEX IF NOT EXISTS user_tenants_tenant_user_idx ON user_tenants(tenant_id, user_id);

-- Ensure at least one tenant exists (legacy installs)
DO $$
DECLARE default_tenant uuid;
BEGIN
  SELECT id INTO default_tenant FROM companies ORDER BY created_at LIMIT 1;
  IF default_tenant IS NULL THEN
    INSERT INTO companies(legal_name, trade_name, cnpj)
    VALUES ('Empresa Padrão', 'Empresa Padrão', '00000000000000')
    RETURNING id INTO default_tenant;
  END IF;

  -- Add tenant_id columns
  ALTER TABLE customers           ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE categories          ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE products            ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE inventory_balances  ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE inventory_movements ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE cash_registers      ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE cash_sessions       ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE sales               ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE sale_items          ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE payments            ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE ledger_entries      ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE expenses            ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE revenues            ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE accounts_payable    ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE accounts_receivable ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE invoices            ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE invoice_xml_files   ADD COLUMN IF NOT EXISTS tenant_id uuid;
  ALTER TABLE audit_logs          ADD COLUMN IF NOT EXISTS tenant_id uuid;

  -- Backfill tenant_id for base tables
  UPDATE customers  SET tenant_id = default_tenant WHERE tenant_id IS NULL;
  UPDATE categories SET tenant_id = default_tenant WHERE tenant_id IS NULL;
  UPDATE products   SET tenant_id = default_tenant WHERE tenant_id IS NULL;

  -- cash: default then propagate
  UPDATE cash_registers SET tenant_id = default_tenant WHERE tenant_id IS NULL;
  UPDATE cash_sessions  SET tenant_id = default_tenant WHERE tenant_id IS NULL;

  -- sales: default then propagate
  UPDATE sales SET tenant_id = default_tenant WHERE tenant_id IS NULL;

  -- finance
  UPDATE ledger_entries SET tenant_id = default_tenant WHERE tenant_id IS NULL;
  UPDATE expenses       SET tenant_id = default_tenant WHERE tenant_id IS NULL;
  UPDATE revenues       SET tenant_id = default_tenant WHERE tenant_id IS NULL;
  UPDATE accounts_payable    SET tenant_id = default_tenant WHERE tenant_id IS NULL;
  UPDATE accounts_receivable SET tenant_id = default_tenant WHERE tenant_id IS NULL;

  -- fiscal
  UPDATE invoices SET tenant_id = company_id WHERE tenant_id IS NULL;

  -- inventory based on products
  UPDATE inventory_balances b
  SET tenant_id = p.tenant_id
  FROM products p
  WHERE b.product_id = p.id AND b.tenant_id IS NULL;

  UPDATE inventory_movements m
  SET tenant_id = p.tenant_id
  FROM products p
  WHERE m.product_id = p.id AND m.tenant_id IS NULL;

  -- propagate down-stream tenant_id
  UPDATE cash_sessions cs
  SET tenant_id = cr.tenant_id
  FROM cash_registers cr
  WHERE cs.cash_register_id = cr.id AND cs.tenant_id IS NULL;

  UPDATE sales s
  SET tenant_id = cs.tenant_id
  FROM cash_sessions cs
  WHERE s.cash_session_id = cs.id AND s.tenant_id IS NULL;

  UPDATE sale_items si
  SET tenant_id = s.tenant_id
  FROM sales s
  WHERE si.sale_id = s.id AND si.tenant_id IS NULL;

  UPDATE payments p
  SET tenant_id = s.tenant_id
  FROM sales s
  WHERE p.sale_id = s.id AND p.tenant_id IS NULL;

  UPDATE invoice_xml_files xf
  SET tenant_id = i.tenant_id
  FROM invoices i
  WHERE xf.invoice_id = i.id AND xf.tenant_id IS NULL;

  UPDATE audit_logs SET tenant_id = default_tenant WHERE tenant_id IS NULL;

  -- Ensure default mapping for existing users
  INSERT INTO user_tenants(user_id, tenant_id)
  SELECT u.id, default_tenant
  FROM users u
  ON CONFLICT DO NOTHING;

END $$;

-- Make tenant_id mandatory (after backfill)
ALTER TABLE customers           ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE categories          ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE products            ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE inventory_balances  ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE inventory_movements ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE cash_registers      ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE cash_sessions       ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE sales               ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE sale_items          ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE payments            ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE ledger_entries      ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE expenses            ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE revenues            ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE accounts_payable    ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE accounts_receivable ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE invoices            ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE invoice_xml_files   ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE audit_logs          ALTER COLUMN tenant_id SET NOT NULL;

-- Unique constraints should be tenant-scoped (MVP)
ALTER TABLE products      DROP CONSTRAINT IF EXISTS products_sku_key;
CREATE UNIQUE INDEX IF NOT EXISTS products_tenant_sku_unique ON products(tenant_id, sku);

ALTER TABLE categories    DROP CONSTRAINT IF EXISTS categories_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS categories_tenant_name_unique ON categories(tenant_id, name);

ALTER TABLE cash_registers DROP CONSTRAINT IF EXISTS cash_registers_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS cash_registers_tenant_name_unique ON cash_registers(tenant_id, name);

DROP INDEX IF EXISTS customers_document_unique;
CREATE UNIQUE INDEX IF NOT EXISTS customers_tenant_document_unique ON customers(tenant_id, document) WHERE document IS NOT NULL;

-- Helpful tenant indexes
CREATE INDEX IF NOT EXISTS products_tenant_name_idx ON products(tenant_id, name);
CREATE INDEX IF NOT EXISTS inventory_balances_tenant_product_idx ON inventory_balances(tenant_id, product_id);
CREATE INDEX IF NOT EXISTS inventory_movements_tenant_product_created_idx ON inventory_movements(tenant_id, product_id, created_at DESC);
CREATE INDEX IF NOT EXISTS cash_sessions_tenant_status_idx ON cash_sessions(tenant_id, status, opened_at DESC);
CREATE INDEX IF NOT EXISTS sales_tenant_created_idx ON sales(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ledger_entries_tenant_created_idx ON ledger_entries(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS invoices_tenant_created_idx ON invoices(tenant_id, created_at DESC);

COMMIT;
