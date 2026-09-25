-- 0017_tenant_relational_integrity.up.sql
BEGIN;

-- A user can belong to more than one independent tenant/company. Membership
-- activation is tenant-scoped so blocking access in one company does not
-- deactivate the global user identity for every other company.
ALTER TABLE user_tenants
  ADD COLUMN IF NOT EXISTS active boolean NOT NULL DEFAULT true;

CREATE INDEX IF NOT EXISTS user_tenants_user_active_created_idx
  ON user_tenants(user_id, active, created_at);

-- Barcode uniqueness must be tenant-scoped: independent stores routinely sell
-- the same EAN/GTIN and must be able to register it independently.
DROP INDEX IF EXISTS products_barcode_unique;
CREATE UNIQUE INDEX IF NOT EXISTS products_tenant_barcode_unique
  ON products(tenant_id, barcode)
  WHERE barcode IS NOT NULL;

-- Base tables created before multi-tenant migration only received a UUID
-- column. Attach those tenant roots to real companies so orphan tenant IDs
-- cannot be persisted.
ALTER TABLE customers
  ADD CONSTRAINT customers_tenant_fk
  FOREIGN KEY (tenant_id) REFERENCES companies(id) ON DELETE RESTRICT;
ALTER TABLE categories
  ADD CONSTRAINT categories_tenant_fk
  FOREIGN KEY (tenant_id) REFERENCES companies(id) ON DELETE RESTRICT;
ALTER TABLE products
  ADD CONSTRAINT products_tenant_fk
  FOREIGN KEY (tenant_id) REFERENCES companies(id) ON DELETE RESTRICT;
ALTER TABLE cash_registers
  ADD CONSTRAINT cash_registers_tenant_fk
  FOREIGN KEY (tenant_id) REFERENCES companies(id) ON DELETE RESTRICT;
ALTER TABLE ledger_entries
  ADD CONSTRAINT ledger_entries_tenant_fk
  FOREIGN KEY (tenant_id) REFERENCES companies(id) ON DELETE RESTRICT;
ALTER TABLE expenses
  ADD CONSTRAINT expenses_tenant_fk
  FOREIGN KEY (tenant_id) REFERENCES companies(id) ON DELETE RESTRICT;
ALTER TABLE revenues
  ADD CONSTRAINT revenues_tenant_fk
  FOREIGN KEY (tenant_id) REFERENCES companies(id) ON DELETE RESTRICT;
ALTER TABLE accounts_payable
  ADD CONSTRAINT accounts_payable_tenant_fk
  FOREIGN KEY (tenant_id) REFERENCES companies(id) ON DELETE RESTRICT;
ALTER TABLE accounts_receivable
  ADD CONSTRAINT accounts_receivable_tenant_fk
  FOREIGN KEY (tenant_id) REFERENCES companies(id) ON DELETE RESTRICT;
ALTER TABLE audit_logs
  ADD CONSTRAINT audit_logs_tenant_fk
  FOREIGN KEY (tenant_id) REFERENCES companies(id) ON DELETE RESTRICT;

-- Composite uniqueness is required by tenant-scoped foreign keys below.
CREATE UNIQUE INDEX IF NOT EXISTS categories_tenant_id_id_unique
  ON categories(tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS customers_tenant_id_id_unique
  ON customers(tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS products_tenant_id_id_unique
  ON products(tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS cash_registers_tenant_id_id_unique
  ON cash_registers(tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS cash_sessions_tenant_id_id_unique
  ON cash_sessions(tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS sales_tenant_id_id_unique
  ON sales(tenant_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS invoices_tenant_id_id_unique
  ON invoices(tenant_id, id);

-- Replace nullable global foreign keys with tenant-scoped equivalents while
-- preserving SET NULL behavior only for the nullable reference column.
ALTER TABLE products
  DROP CONSTRAINT IF EXISTS products_category_id_fkey,
  ADD CONSTRAINT products_tenant_category_fk
  FOREIGN KEY (tenant_id, category_id)
  REFERENCES categories(tenant_id, id)
  ON DELETE SET NULL (category_id);

ALTER TABLE sales
  DROP CONSTRAINT IF EXISTS sales_customer_id_fkey,
  ADD CONSTRAINT sales_tenant_customer_fk
  FOREIGN KEY (tenant_id, customer_id)
  REFERENCES customers(tenant_id, id)
  ON DELETE SET NULL (customer_id);

-- Prevent a row carrying tenant A from referencing an owning row from tenant B.
ALTER TABLE inventory_balances
  ADD CONSTRAINT inventory_balances_tenant_product_fk
  FOREIGN KEY (tenant_id, product_id)
  REFERENCES products(tenant_id, id)
  ON DELETE CASCADE;

ALTER TABLE inventory_movements
  ADD CONSTRAINT inventory_movements_tenant_product_fk
  FOREIGN KEY (tenant_id, product_id)
  REFERENCES products(tenant_id, id)
  ON DELETE RESTRICT;

ALTER TABLE cash_sessions
  ADD CONSTRAINT cash_sessions_tenant_register_fk
  FOREIGN KEY (tenant_id, cash_register_id)
  REFERENCES cash_registers(tenant_id, id)
  ON DELETE RESTRICT;

ALTER TABLE sales
  ADD CONSTRAINT sales_tenant_cash_session_fk
  FOREIGN KEY (tenant_id, cash_session_id)
  REFERENCES cash_sessions(tenant_id, id)
  ON DELETE RESTRICT;

ALTER TABLE sale_items
  ADD CONSTRAINT sale_items_tenant_sale_fk
  FOREIGN KEY (tenant_id, sale_id)
  REFERENCES sales(tenant_id, id)
  ON DELETE CASCADE,
  ADD CONSTRAINT sale_items_tenant_product_fk
  FOREIGN KEY (tenant_id, product_id)
  REFERENCES products(tenant_id, id)
  ON DELETE RESTRICT;

ALTER TABLE payments
  ADD CONSTRAINT payments_tenant_sale_fk
  FOREIGN KEY (tenant_id, sale_id)
  REFERENCES sales(tenant_id, id)
  ON DELETE CASCADE;

ALTER TABLE ledger_entries
  DROP CONSTRAINT IF EXISTS ledger_entries_sale_id_fkey,
  DROP CONSTRAINT IF EXISTS ledger_entries_cash_session_id_fkey,
  ADD CONSTRAINT ledger_entries_tenant_sale_fk
  FOREIGN KEY (tenant_id, sale_id)
  REFERENCES sales(tenant_id, id)
  ON DELETE SET NULL (sale_id),
  ADD CONSTRAINT ledger_entries_tenant_cash_session_fk
  FOREIGN KEY (tenant_id, cash_session_id)
  REFERENCES cash_sessions(tenant_id, id)
  ON DELETE SET NULL (cash_session_id);

ALTER TABLE invoices
  ADD CONSTRAINT invoices_tenant_sale_fk
  FOREIGN KEY (tenant_id, sale_id)
  REFERENCES sales(tenant_id, id)
  ON DELETE RESTRICT,
  ADD CONSTRAINT invoices_company_matches_tenant
  CHECK (tenant_id = company_id);

ALTER TABLE invoice_xml_files
  ADD CONSTRAINT invoice_xml_files_tenant_invoice_fk
  FOREIGN KEY (tenant_id, invoice_id)
  REFERENCES invoices(tenant_id, id)
  ON DELETE CASCADE;

ALTER TABLE idempotency_keys
  ADD CONSTRAINT idempotency_keys_tenant_sale_fk
  FOREIGN KEY (tenant_id, sale_id)
  REFERENCES sales(tenant_id, id)
  ON DELETE CASCADE;

ALTER TABLE cash_movements
  ADD CONSTRAINT cash_movements_tenant_session_fk
  FOREIGN KEY (tenant_id, cash_session_id)
  REFERENCES cash_sessions(tenant_id, id)
  ON DELETE RESTRICT;

ALTER TABLE cash_session_reconciliations
  ADD CONSTRAINT cash_reconciliations_tenant_session_fk
  FOREIGN KEY (tenant_id, cash_session_id)
  REFERENCES cash_sessions(tenant_id, id)
  ON DELETE CASCADE;

ALTER TABLE user_tenant_roles
  ADD CONSTRAINT user_tenant_roles_membership_fk
  FOREIGN KEY (user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE CASCADE;

-- Historical actor references must also prove that the user belonged to the
-- tenant that owns the business row. Membership rows are deactivated rather
-- than deleted, preserving this legal/audit history.
ALTER TABLE inventory_movements
  ADD CONSTRAINT inventory_movements_actor_membership_fk
  FOREIGN KEY (actor_user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE SET NULL (actor_user_id);

ALTER TABLE cash_sessions
  ADD CONSTRAINT cash_sessions_opened_by_membership_fk
  FOREIGN KEY (opened_by_user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE RESTRICT,
  ADD CONSTRAINT cash_sessions_closed_by_membership_fk
  FOREIGN KEY (closed_by_user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE SET NULL (closed_by_user_id);

ALTER TABLE sales
  ADD CONSTRAINT sales_created_by_membership_fk
  FOREIGN KEY (created_by_user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE RESTRICT;

ALTER TABLE ledger_entries
  ADD CONSTRAINT ledger_entries_created_by_membership_fk
  FOREIGN KEY (created_by_user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE SET NULL (created_by_user_id);

ALTER TABLE expenses
  ADD CONSTRAINT expenses_created_by_membership_fk
  FOREIGN KEY (created_by_user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE SET NULL (created_by_user_id);

ALTER TABLE revenues
  ADD CONSTRAINT revenues_created_by_membership_fk
  FOREIGN KEY (created_by_user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE SET NULL (created_by_user_id);

ALTER TABLE invoices
  ADD CONSTRAINT invoices_created_by_membership_fk
  FOREIGN KEY (created_by_user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE SET NULL (created_by_user_id);

ALTER TABLE cash_movements
  ADD CONSTRAINT cash_movements_created_by_membership_fk
  FOREIGN KEY (created_by_user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE SET NULL (created_by_user_id);

ALTER TABLE data_subject_requests
  ADD CONSTRAINT data_subject_requests_created_by_membership_fk
  FOREIGN KEY (created_by_user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE SET NULL (created_by_user_id);

ALTER TABLE audit_logs
  ADD CONSTRAINT audit_logs_actor_membership_fk
  FOREIGN KEY (actor_user_id, tenant_id)
  REFERENCES user_tenants(user_id, tenant_id)
  ON DELETE SET NULL (actor_user_id);

COMMIT;
