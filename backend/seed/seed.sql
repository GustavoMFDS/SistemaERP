-- Seed MVP
-- Development/demo seed only. It creates default users with known passwords.
-- Run only with psql -v ALLOW_DEMO_SEED=1.

\if :{?ALLOW_DEMO_SEED}
\if :ALLOW_DEMO_SEED
\else
\echo 'Refusing to run demo seed unless ALLOW_DEMO_SEED=1'
SELECT 1/0;
\quit
\endif
\else
\echo 'Refusing to run demo seed without -v ALLOW_DEMO_SEED=1'
SELECT 1/0;
\quit
\endif

BEGIN;

-- Ensure a deterministic default tenant and capture its ID for tenant-scoped seeds.
-- We use psql variables via \gset so the rest of the file can reference :'tenant_id'.
WITH ins AS (
  INSERT INTO companies (legal_name, trade_name, cnpj, ie, crt, created_at)
  SELECT 'Empresa Exemplo LTDA', 'Loja Exemplo', '00000000000000', 'ISENTO', '1', now()
  WHERE NOT EXISTS (SELECT 1 FROM companies WHERE cnpj='00000000000000')
  RETURNING id, created_at
)
SELECT id AS tenant_id, created_at AS tenant_created_at
FROM ins
UNION ALL
SELECT id AS tenant_id, created_at AS tenant_created_at
FROM companies
WHERE cnpj='00000000000000'
ORDER BY tenant_created_at
LIMIT 1
\gset

INSERT INTO roles (id, name) VALUES
  (gen_random_uuid(), 'admin'),
  (gen_random_uuid(), 'manager'),
  (gen_random_uuid(), 'cashier')
ON CONFLICT (name) DO NOTHING;

-- permissions
INSERT INTO permissions (id, code, description) VALUES
  (gen_random_uuid(), 'product:read', 'Listar/visualizar produtos'),
  (gen_random_uuid(), 'product:write', 'Criar/editar produtos'),
  (gen_random_uuid(), 'inventory:read', 'Consultar estoque e movimentações'),
  (gen_random_uuid(), 'inventory:adjust', 'Ajuste manual de estoque'),
  (gen_random_uuid(), 'procurement:read', 'Consultar fornecedores e compras'),
  (gen_random_uuid(), 'procurement:write', 'Criar e alterar fornecedores e compras'),
  (gen_random_uuid(), 'procurement:receive', 'Receber compras e atualizar estoque'),
  (gen_random_uuid(), 'cash:open', 'Abrir caixa'),
  (gen_random_uuid(), 'cash:move', 'Registrar sangria e suprimento de caixa'),
  (gen_random_uuid(), 'cash:close', 'Fechar caixa'),
  (gen_random_uuid(), 'sale:read', 'Consultar vendas'),
  (gen_random_uuid(), 'sale:write', 'Criar/finalizar venda'),
  (gen_random_uuid(), 'sale:cancel', 'Cancelar venda'),
  (gen_random_uuid(), 'sale:return', 'Registrar devolucoes e trocas'),
  (gen_random_uuid(), 'sale:discount', 'Aplicar desconto no PDV'),
  (gen_random_uuid(), 'finance:read', 'Consultar financeiro'),
  (gen_random_uuid(), 'finance:reconcile', 'Conciliar pagamentos e liquidar reembolsos'),
  (gen_random_uuid(), 'invoice:generate', 'Gerar XML NF-e'),
  (gen_random_uuid(), 'invoice:read', 'Consultar XML NF-e'),
  (gen_random_uuid(), 'privacy:read', 'Consultar requisicoes LGPD e consentimentos'),
  (gen_random_uuid(), 'privacy:write', 'Processar requisicoes LGPD e consentimentos'),
  (gen_random_uuid(), 'audit:read', 'Consultar logs de auditoria')
ON CONFLICT (code) DO NOTHING;

-- Role permissions
-- admin: all
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name='admin'
ON CONFLICT DO NOTHING;

-- manager: most
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'product:read','product:write','inventory:read','inventory:adjust',
  'procurement:read','procurement:write','procurement:receive',
  'cash:open','cash:move','cash:close','sale:read','sale:write','sale:cancel','sale:return','sale:discount',
  'finance:read','finance:reconcile','invoice:generate','invoice:read'
)
WHERE r.name='manager'
ON CONFLICT DO NOTHING;

-- cashier: operar caixa e venda
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN (
  'product:read','inventory:read','cash:open','cash:close','sale:read','sale:write'
)
WHERE r.name='cashier'
ON CONFLICT DO NOTHING;

-- company (emitente) mínima
-- ensured above (tenant_id captured in :'tenant_id')

-- users (senha: admin123)
-- O hash é gerado no próprio Postgres usando pgcrypto.crypt() (bcrypt).
INSERT INTO users (id, email, name, password_hash, active, created_at)
VALUES
  (gen_random_uuid(), 'admin@sistema.local', 'Admin', crypt('admin123', gen_salt('bf', 10)), true, now()),
  (gen_random_uuid(), 'gerente@sistema.local', 'Gerente', crypt('admin123', gen_salt('bf', 10)), true, now()),
  (gen_random_uuid(), 'caixa@sistema.local', 'Operador Caixa', crypt('admin123', gen_salt('bf', 10)), true, now())
ON CONFLICT (email) DO NOTHING;

-- map users to default tenant (required after multi-tenant migration)
INSERT INTO user_tenants(user_id, tenant_id)
SELECT u.id, :'tenant_id'::uuid
FROM users u
WHERE u.email IN ('admin@sistema.local','gerente@sistema.local','caixa@sistema.local')
ON CONFLICT DO NOTHING;

-- map roles
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u JOIN roles r ON r.name='admin' WHERE u.email='admin@sistema.local'
ON CONFLICT DO NOTHING;
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u JOIN roles r ON r.name='manager' WHERE u.email='gerente@sistema.local'
ON CONFLICT DO NOTHING;
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u JOIN roles r ON r.name='cashier' WHERE u.email='caixa@sistema.local'
ON CONFLICT DO NOTHING;

INSERT INTO user_tenant_roles(user_id, tenant_id, role_id)
SELECT u.id, :'tenant_id'::uuid, r.id
FROM users u
JOIN roles r ON (
  (u.email='admin@sistema.local' AND r.name='admin') OR
  (u.email='gerente@sistema.local' AND r.name='manager') OR
  (u.email='caixa@sistema.local' AND r.name='cashier')
)
WHERE u.email IN ('admin@sistema.local','gerente@sistema.local','caixa@sistema.local')
ON CONFLICT DO NOTHING;

-- categories + products
INSERT INTO categories (id, tenant_id, name, created_at) VALUES
  (gen_random_uuid(), :'tenant_id'::uuid, 'Bebidas', now()),
  (gen_random_uuid(), :'tenant_id'::uuid, 'Mercearia', now())
ON CONFLICT (tenant_id, name) DO NOTHING;

INSERT INTO products (id, tenant_id, category_id, sku, barcode, name, description, unit, cost_price, price_cash, promo_price, min_stock, active, ncm, created_at)
SELECT gen_random_uuid(), :'tenant_id'::uuid, c.id, 'SKU-COCA-2L', '7890000000000', 'Coca-Cola 2L', '', 'UN', 7.00, 10.90, NULL, 5, true, '22021000', now()
FROM categories c
WHERE c.tenant_id = :'tenant_id'::uuid AND c.name='Bebidas'
ON CONFLICT (tenant_id, sku) DO NOTHING;

-- initial stock balance + movement
INSERT INTO inventory_balances (product_id, tenant_id, qty_on_hand, updated_at)
SELECT p.id, p.tenant_id, 20, now()
FROM products p
WHERE p.tenant_id = :'tenant_id'::uuid AND p.sku='SKU-COCA-2L'
ON CONFLICT (product_id) DO NOTHING;

COMMIT;
