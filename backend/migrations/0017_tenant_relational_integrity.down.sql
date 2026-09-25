-- 0017_tenant_relational_integrity.down.sql
BEGIN;

DO $
BEGIN
  IF EXISTS (
    SELECT 1
    FROM user_tenants
    WHERE active=false
  ) THEN
    RAISE EXCEPTION 'cannot rollback migration 0017 while inactive tenant memberships exist; reconcile or reactivate them first';
  END IF;

  IF EXISTS (
    SELECT barcode
    FROM products
    WHERE barcode IS NOT NULL
    GROUP BY barcode
    HAVING count(*) > 1
  ) THEN
    RAISE EXCEPTION 'cannot rollback migration 0017 while the same barcode exists in multiple tenants; reconcile duplicate barcodes first';
  END IF;
END $;

ALTER TABLE sales
  DROP CONSTRAINT IF EXISTS sales_tenant_customer_fk,
  ADD CONSTRAINT sales_customer_id_fkey
  FOREIGN KEY (customer_id)
  REFERENCES customers(id)
  ON DELETE SET NULL;
ALTER TABLE products
  DROP CONSTRAINT IF EXISTS products_tenant_category_fk,
  ADD CONSTRAINT products_category_id_fkey
  FOREIGN KEY (category_id)
  REFERENCES categories(id)
  ON DELETE SET NULL;

ALTER TABLE audit_logs DROP CONSTRAINT IF EXISTS audit_logs_tenant_fk;
ALTER TABLE accounts_receivable DROP CONSTRAINT IF EXISTS accounts_receivable_tenant_fk;
ALTER TABLE accounts_payable DROP CONSTRAINT IF EXISTS accounts_payable_tenant_fk;
ALTER TABLE revenues DROP CONSTRAINT IF EXISTS revenues_tenant_fk;
ALTER TABLE expenses DROP CONSTRAINT IF EXISTS expenses_tenant_fk;
ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_entries_tenant_fk;
ALTER TABLE cash_registers DROP CONSTRAINT IF EXISTS cash_registers_tenant_fk;
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_tenant_fk;
ALTER TABLE categories DROP CONSTRAINT IF EXISTS categories_tenant_fk;
ALTER TABLE customers DROP CONSTRAINT IF EXISTS customers_tenant_fk;

DROP INDEX IF EXISTS products_tenant_barcode_unique;
CREATE UNIQUE INDEX IF NOT EXISTS products_barcode_unique
  ON products(barcode)
  WHERE barcode IS NOT NULL;

ALTER TABLE audit_logs
  DROP CONSTRAINT IF EXISTS audit_logs_actor_membership_fk;
ALTER TABLE data_subject_requests
  DROP CONSTRAINT IF EXISTS data_subject_requests_created_by_membership_fk;
ALTER TABLE cash_movements
  DROP CONSTRAINT IF EXISTS cash_movements_created_by_membership_fk;
ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_created_by_membership_fk;
ALTER TABLE revenues
  DROP CONSTRAINT IF EXISTS revenues_created_by_membership_fk;
ALTER TABLE expenses
  DROP CONSTRAINT IF EXISTS expenses_created_by_membership_fk;
ALTER TABLE ledger_entries
  DROP CONSTRAINT IF EXISTS ledger_entries_created_by_membership_fk;
ALTER TABLE sales
  DROP CONSTRAINT IF EXISTS sales_created_by_membership_fk;
ALTER TABLE cash_sessions
  DROP CONSTRAINT IF EXISTS cash_sessions_closed_by_membership_fk,
  DROP CONSTRAINT IF EXISTS cash_sessions_opened_by_membership_fk;
ALTER TABLE inventory_movements
  DROP CONSTRAINT IF EXISTS inventory_movements_actor_membership_fk;

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
ALTER TABLE ledger_entries
  DROP CONSTRAINT IF EXISTS ledger_entries_tenant_cash_session_fk,
  DROP CONSTRAINT IF EXISTS ledger_entries_tenant_sale_fk,
  ADD CONSTRAINT ledger_entries_sale_id_fkey
  FOREIGN KEY (sale_id)
  REFERENCES sales(id)
  ON DELETE SET NULL,
  ADD CONSTRAINT ledger_entries_cash_session_id_fkey
  FOREIGN KEY (cash_session_id)
  REFERENCES cash_sessions(id)
  ON DELETE SET NULL;
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
DROP INDEX IF EXISTS customers_tenant_id_id_unique;
DROP INDEX IF EXISTS categories_tenant_id_id_unique;
DROP INDEX IF EXISTS user_tenants_user_active_created_idx;

ALTER TABLE user_tenants
  DROP COLUMN IF EXISTS active;

COMMIT;
