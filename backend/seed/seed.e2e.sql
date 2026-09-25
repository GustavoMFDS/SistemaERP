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


-- Shared admin used to prove explicit tenant switching. The default tenant
-- membership was created first by seed.sql, so login remains deterministic in
-- tenant A until the user explicitly switches.
INSERT INTO user_tenants(user_id, tenant_id)
SELECT u.id, :'tenant_b_id'::uuid
FROM users u
WHERE u.email='admin@sistema.local'
ON CONFLICT (user_id, tenant_id) DO UPDATE SET active=true;

INSERT INTO user_tenant_roles(user_id, tenant_id, role_id)
SELECT u.id, :'tenant_b_id'::uuid, r.id
FROM users u
JOIN roles r ON r.name='admin'
WHERE u.email='admin@sistema.local'
ON CONFLICT DO NOTHING;

INSERT INTO categories(id, tenant_id, name, created_at)
VALUES (gen_random_uuid(), :'tenant_b_id'::uuid, 'Mercearia E2E B', now())
ON CONFLICT (tenant_id, name) DO NOTHING;

-- Reuse the same EAN as tenant A intentionally: barcode uniqueness is
-- tenant-scoped after migration 0017.
INSERT INTO products(
  id, tenant_id, category_id, sku, barcode, name, description, unit,
  cost_price, price_cash, promo_price, min_stock, active, created_at
)
SELECT
  gen_random_uuid(), :'tenant_b_id'::uuid, c.id, 'SKU-E2E-B-ARROZ',
  '7890000000000', 'Arroz E2E Tenant B', '', 'UN',
  12.00, 18.50, NULL, 2, true, now()
FROM categories c
WHERE c.tenant_id=:'tenant_b_id'::uuid AND c.name='Mercearia E2E B'
ON CONFLICT (tenant_id, sku) DO NOTHING;

INSERT INTO inventory_balances(product_id, tenant_id, qty_on_hand, updated_at)
SELECT p.id, p.tenant_id, 7, now()
FROM products p
WHERE p.tenant_id=:'tenant_b_id'::uuid AND p.sku='SKU-E2E-B-ARROZ'
ON CONFLICT (product_id) DO UPDATE
SET tenant_id=EXCLUDED.tenant_id, qty_on_hand=EXCLUDED.qty_on_hand, updated_at=now();

COMMIT;
