-- 0012_tenant_scoped_roles.down.sql

BEGIN;

DROP INDEX IF EXISTS user_tenant_roles_role_idx;
DROP INDEX IF EXISTS user_tenant_roles_tenant_user_idx;
DROP TABLE IF EXISTS user_tenant_roles;

COMMIT;
