-- 0021_pdv_operational_permissions.down.sql

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code='sale:discount');
DELETE FROM permissions WHERE code='sale:discount';

COMMIT;
