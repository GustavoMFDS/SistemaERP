-- 0019_sale_returns.down.sql

BEGIN;

DROP TABLE IF EXISTS return_idempotency_keys;
DROP TABLE IF EXISTS sale_return_items;
DROP TABLE IF EXISTS sale_returns;

DROP INDEX IF EXISTS sale_items_tenant_id_id_unique;
DROP INDEX IF EXISTS sales_tenant_id_id_unique;

COMMIT;
