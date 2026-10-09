BEGIN;
-- Rolling back while anyone is suspended would silently restore their
-- access under the old user_tenants schema. Block that unsafe downgrade.
DO $
BEGIN
  IF EXISTS (SELECT 1 FROM user_tenants WHERE active=false) THEN
    RAISE EXCEPTION 'unsafe rollback: inactive tenant memberships must remain revoked';
  END IF;
END $;
DROP TABLE IF EXISTS staff_invitations;
DELETE FROM role_permissions rp USING permissions p
WHERE rp.permission_id=p.id AND p.code='team:manage';
DELETE FROM permissions WHERE code='team:manage';
ALTER TABLE user_tenants DROP COLUMN IF EXISTS active;
COMMIT;
