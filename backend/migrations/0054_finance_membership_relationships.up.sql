-- 0054: protect financial links and membership-scoped role grants.
-- Validate existing rows; never reassign or silently remove inconsistent data.
BEGIN;
ALTER TABLE ledger_entries ADD CONSTRAINT core54_ledger_sale_fk
  FOREIGN KEY (tenant_id, sale_id) REFERENCES sales(tenant_id, id) ON DELETE SET NULL (sale_id);
ALTER TABLE ledger_entries ADD CONSTRAINT core54_ledger_cash_fk
  FOREIGN KEY (tenant_id, cash_session_id) REFERENCES cash_sessions(tenant_id, id) ON DELETE SET NULL (cash_session_id);
ALTER TABLE idempotency_keys ADD CONSTRAINT core54_idempotency_sale_fk
  FOREIGN KEY (tenant_id, sale_id) REFERENCES sales(tenant_id, id) ON DELETE RESTRICT;
ALTER TABLE user_tenant_roles ADD CONSTRAINT core54_roles_membership_fk
  FOREIGN KEY (user_id, tenant_id) REFERENCES user_tenants(user_id, tenant_id) ON DELETE CASCADE;
CREATE INDEX core54_ledger_sale_idx ON ledger_entries(tenant_id, sale_id) WHERE sale_id IS NOT NULL;
CREATE INDEX core54_ledger_cash_idx ON ledger_entries(tenant_id, cash_session_id) WHERE cash_session_id IS NOT NULL;
CREATE INDEX core54_idempotency_sale_idx ON idempotency_keys(tenant_id, sale_id);
COMMIT;
