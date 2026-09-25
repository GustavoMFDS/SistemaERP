-- 0017_tenant_relational_integrity.down.sql
BEGIN;

ALTER TABLE user_tenant_roles
  DROP CONSTRAINT IF EXISTS user_tenant_roles_membership_fk;
ALTER TABLE cash_session_reconciliations
  DROP CONSTRAINT IF EXISTS cash_reconciliations_tenant_session_fk;
ALTER TABLE cash_movements
  DROP CONSTRAINT IF EXISTS cash_movements_tenant_session_fk;
ALTER TABLE idempotency_keys
  DROP CONSTRAINT IF EXISTS idempotency_keys_tenant_sale_fk;
ALTER TABLE invoice_xml_files
  DROP CONSTRAINT IF EXISTS invoice_xml_files_tenant_invoice_fk;
ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_company_matches_tenant,
  DROP CONSTRAINT IF EXISTS invoices_tenant_sale_fk;
ALTER TABLE payments
  DROP CONSTRAINT IF EXISTS payments_tenant_sale_fk;
ALTER TABLE sale_items
  DROP CONSTRAINT IF EXISTS sale_items_tenant_product_fk,
  DROP CONSTRAINT IF EXISTS sale_items_tenant_sale_fk;
ALTER TABLE sales
  DROP CONSTRAINT IF EXISTS sales_tenant_cash_session_fk;
ALTER TABLE cash_sessions
  DROP CONSTRAINT IF EXISTS cash_sessions_tenant_register_fk;
ALTER TABLE inventory_movements
  DROP CONSTRAINT IF EXISTS inventory_movements_tenant_product_fk;
ALTER TABLE inventory_balances
  DROP CONSTRAINT IF EXISTS inventory_balances_tenant_product_fk;

DROP INDEX IF EXISTS invoices_tenant_id_id_unique;
DROP INDEX IF EXISTS sales_tenant_id_id_unique;
DROP INDEX IF EXISTS cash_sessions_tenant_id_id_unique;
DROP INDEX IF EXISTS cash_registers_tenant_id_id_unique;
DROP INDEX IF EXISTS products_tenant_id_id_unique;
DROP INDEX IF EXISTS user_tenants_user_active_created_idx;

ALTER TABLE user_tenants
  DROP COLUMN IF EXISTS active;

COMMIT;
