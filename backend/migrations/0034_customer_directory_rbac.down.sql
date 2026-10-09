BEGIN;
DROP INDEX IF EXISTS customers_tenant_created_id_idx;
DELETE FROM role_permissions rp USING permissions p
WHERE rp.permission_id=p.id AND p.code IN ('customer:read','customer:write');
DELETE FROM permissions WHERE code IN ('customer:read','customer:write');
COMMIT;
