-- 0003_multi_tenant.down.sql

BEGIN;

-- Drop tenant-scoped indexes
DROP INDEX IF EXISTS invoices_tenant_created_idx;
DROP INDEX IF EXISTS ledger_entries_tenant_created_idx;
DROP INDEX IF EXISTS sales_tenant_created_idx;
DROP INDEX IF EXISTS cash_sessions_tenant_status_idx;
DROP INDEX IF EXISTS inventory_movements_tenant_product_created_idx;
DROP INDEX IF EXISTS inventory_balances_tenant_product_idx;
DROP INDEX IF EXISTS products_tenant_name_idx;

DROP INDEX IF EXISTS customers_tenant_document_unique;
DROP INDEX IF EXISTS cash_registers_tenant_name_unique;
DROP INDEX IF EXISTS categories_tenant_name_unique;
DROP INDEX IF EXISTS products_tenant_sku_unique;

-- Restore legacy uniqueness (best-effort)
ALTER TABLE products      ADD CONSTRAINT products_sku_key UNIQUE (sku);
ALTER TABLE categories    ADD CONSTRAINT categories_name_key UNIQUE (name);
ALTER TABLE cash_registers ADD CONSTRAINT cash_registers_name_key UNIQUE (name);
CREATE UNIQUE INDEX IF NOT EXISTS customers_document_unique ON customers(document) WHERE document IS NOT NULL;

-- Drop tenant_id columns
ALTER TABLE audit_logs          DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE invoice_xml_files   DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE invoices            DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE accounts_receivable DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE accounts_payable    DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE revenues            DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE expenses            DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE ledger_entries      DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE payments            DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE sale_items          DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE sales               DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE cash_sessions       DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE cash_registers      DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE inventory_movements DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE inventory_balances  DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE products            DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE categories          DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE customers           DROP COLUMN IF EXISTS tenant_id;

-- Drop join table
DROP TABLE IF EXISTS user_tenants;

COMMIT;
