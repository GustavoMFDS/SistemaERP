-- 0009_privacy_permissions.down.sql

BEGIN;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.code IN ('privacy:read', 'privacy:write');

DELETE FROM permissions
WHERE code IN ('privacy:read', 'privacy:write');

COMMIT;
