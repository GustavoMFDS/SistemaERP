-- 0018_procurement.down.sql

BEGIN;

DROP TABLE IF EXISTS procurement_idempotency_keys;

DROP INDEX IF EXISTS accounts_payable_tenant_purchase_unique;
ALTER TABLE accounts_payable
  DROP COLUMN IF EXISTS purchase_id,
  DROP COLUMN IF EXISTS supplier_id;

DROP TABLE IF EXISTS purchase_receipt_items;
DROP TABLE IF EXISTS purchase_receipts;
DROP TABLE IF EXISTS purchase_items;
DROP TABLE IF EXISTS purchases;
DROP TABLE IF EXISTS suppliers;

DROP INDEX IF EXISTS products_tenant_id_id_unique;

DELETE FROM role_permissions
WHERE permission_id IN (
  SELECT id FROM permissions
  WHERE code IN ('procurement:read','procurement:write','procurement:receive')
);
DELETE FROM permissions
WHERE code IN ('procurement:read','procurement:write','procurement:receive');

COMMIT;
