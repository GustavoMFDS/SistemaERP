-- 0016_procurement.down.sql

BEGIN;

DROP INDEX IF EXISTS accounts_payable_tenant_purchase_unique;
ALTER TABLE accounts_payable
  DROP COLUMN IF EXISTS purchase_id,
  DROP COLUMN IF EXISTS supplier_id;

DROP TABLE IF EXISTS purchase_receipt_items;
DROP TABLE IF EXISTS purchase_receipts;
DROP TABLE IF EXISTS purchase_items;
DROP TABLE IF EXISTS purchases;
DROP TABLE IF EXISTS suppliers;

COMMIT;
