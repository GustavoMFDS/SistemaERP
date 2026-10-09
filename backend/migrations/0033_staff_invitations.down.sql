BEGIN;
DROP TABLE IF EXISTS staff_invitations;
DELETE FROM role_permissions rp USING permissions p
WHERE rp.permission_id=p.id AND p.code='team:manage';
DELETE FROM permissions WHERE code='team:manage';
ALTER TABLE user_tenants DROP COLUMN IF EXISTS active;
COMMIT;
