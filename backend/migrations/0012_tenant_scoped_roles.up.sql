-- 0012_tenant_scoped_roles.up.sql

BEGIN;

CREATE TABLE IF NOT EXISTS user_tenant_roles (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, tenant_id, role_id)
);

INSERT INTO user_tenant_roles(user_id, tenant_id, role_id, created_at)
SELECT ur.user_id, ut.tenant_id, ur.role_id, ur.created_at
FROM user_roles ur
JOIN user_tenants ut ON ut.user_id = ur.user_id
ON CONFLICT DO NOTHING;

CREATE INDEX IF NOT EXISTS user_tenant_roles_tenant_user_idx
  ON user_tenant_roles(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS user_tenant_roles_role_idx
  ON user_tenant_roles(role_id);

COMMIT;
