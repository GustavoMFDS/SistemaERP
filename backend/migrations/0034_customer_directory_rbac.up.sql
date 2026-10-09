-- Customers already exist in the tenant-scoped schema. This migration
-- only adds least-privilege access for a safe admin contact directory.
BEGIN;
INSERT INTO permissions(id,code,description) VALUES
  (gen_random_uuid(),'customer:read','Consultar clientes da propria empresa'),
  (gen_random_uuid(),'customer:write','Cadastrar e corrigir clientes da propria empresa')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions(role_id,permission_id)
SELECT r.id,p.id FROM roles r CROSS JOIN permissions p
WHERE r.name IN ('admin','manager') AND p.code IN ('customer:read','customer:write')
ON CONFLICT DO NOTHING;

CREATE INDEX IF NOT EXISTS customers_tenant_created_id_idx
  ON customers(tenant_id, created_at DESC, id DESC);

COMMIT;
