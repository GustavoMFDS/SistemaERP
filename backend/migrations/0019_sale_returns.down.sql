-- 0019_sale_returns.down.sql

BEGIN;

DROP TABLE IF EXISTS return_idempotency_keys;
DROP TABLE IF EXISTS sale_return_items;
DROP TABLE IF EXISTS sale_returns;

DROP INDEX IF EXISTS sale_items_tenant_id_id_unique;
DROP INDEX IF EXISTS sales_tenant_id_id_unique;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code='sale:return');
DELETE FROM permissions WHERE code='sale:return';

COMMIT;
