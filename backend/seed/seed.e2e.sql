-- Additional deterministic tenant used only by browser/integration E2E.
BEGIN;

WITH ins AS (
  INSERT INTO companies (legal_name, trade_name, cnpj, ie, crt, created_at)
  SELECT 'Empresa E2E Tenant B LTDA', 'Loja E2E B', '11111111111111', 'ISENTO', '1', now()
  WHERE NOT EXISTS (SELECT 1 FROM companies WHERE cnpj='11111111111111')
  RETURNING id, created_at
)
SELECT id AS tenant_b_id, created_at AS tenant_b_created_at
FROM ins
UNION ALL
SELECT id AS tenant_b_id, created_at AS tenant_b_created_at
FROM companies
WHERE cnpj='11111111111111'
ORDER BY tenant_b_created_at
LIMIT 1
\gset

INSERT INTO users (id, email, name, password_hash, active, created_at)
VALUES (
  gen_random_uuid(),
  'admin-b@sistema.local',
  'Admin Tenant B',
  crypt('admin123', gen_salt('bf', 10)),
  true,
  now()
)
ON CONFLICT (email) DO NOTHING;

INSERT INTO user_tenants(user_id, tenant_id)
SELECT u.id, :'tenant_b_id'::uuid
FROM users u
WHERE u.email='admin-b@sistema.local'
ON CONFLICT DO NOTHING;

INSERT INTO user_roles(user_id, role_id)
SELECT u.id, r.id
FROM users u
JOIN roles r ON r.name='admin'
WHERE u.email='admin-b@sistema.local'
ON CONFLICT DO NOTHING;

INSERT INTO user_tenant_roles(user_id, tenant_id, role_id)
SELECT u.id, :'tenant_b_id'::uuid, r.id
FROM users u
JOIN roles r ON r.name='admin'
WHERE u.email='admin-b@sistema.local'
ON CONFLICT DO NOTHING;

COMMIT;
