-- 0017_tenant_relational_integrity.up.sql
BEGIN;

-- A user can belong to more than one independent tenant/company. Membership
-- activation is tenant-scoped so blocking access in one company does not
-- deactivate the global user identity for every other company.
ALTER TABLE user_tenants
  ADD COLUMN IF NOT EXISTS active boolean NOT NULL DEFAULT true;

CREATE INDEX IF NOT EXISTS user_tenants_user_active_created_idx
  ON user_tenants(user_id, active, created_at);

-- Composite uniqueness is required by tenant-scoped foreign keys below.
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

COMMIT;
