-- Remove only the added constraints and indexes; never modify row data.
BEGIN;
ALTER TABLE user_tenant_roles DROP CONSTRAINT core54_roles_membership_fk;
ALTER TABLE idempotency_keys DROP CONSTRAINT core54_idempotency_sale_fk;
ALTER TABLE ledger_entries DROP CONSTRAINT core54_ledger_cash_fk;
ALTER TABLE ledger_entries DROP CONSTRAINT core54_ledger_sale_fk;
DROP INDEX core54_idempotency_sale_idx;
DROP INDEX core54_ledger_cash_idx;
DROP INDEX core54_ledger_sale_idx;
COMMIT;
