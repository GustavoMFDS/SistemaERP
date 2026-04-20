-- Seed MVP

BEGIN;

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
  (gen_random_uuid(), 'cash:open', 'Abrir caixa'),
  (gen_random_uuid(), 'cash:close', 'Fechar caixa'),
  (gen_random_uuid(), 'sale:read', 'Consultar vendas'),
  (gen_random_uuid(), 'sale:write', 'Criar/finalizar venda'),
  (gen_random_uuid(), 'sale:cancel', 'Cancelar venda'),
  (gen_random_uuid(), 'finance:read', 'Consultar financeiro'),
  (gen_random_uuid(), 'invoice:generate', 'Gerar XML NF-e'),
  (gen_random_uuid(), 'invoice:read', 'Consultar XML NF-e')
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
  'cash:open','cash:close','sale:read','sale:write','sale:cancel',
  'finance:read','invoice:generate','invoice:read'
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
INSERT INTO companies (id, legal_name, trade_name, cnpj, ie, crt, created_at)
VALUES (gen_random_uuid(), 'Empresa Exemplo LTDA', 'Loja Exemplo', '00000000000000', 'ISENTO', '1', now())
ON CONFLICT DO NOTHING;

-- users (senha: admin123)
-- O hash é gerado no próprio Postgres usando pgcrypto.crypt() (bcrypt).
INSERT INTO users (id, email, name, password_hash, active, created_at)
VALUES
  (gen_random_uuid(), 'admin@sistema.local', 'Admin', crypt('admin123', gen_salt('bf', 10)), true, now()),
  (gen_random_uuid(), 'gerente@sistema.local', 'Gerente', crypt('admin123', gen_salt('bf', 10)), true, now()),
  (gen_random_uuid(), 'caixa@sistema.local', 'Operador Caixa', crypt('admin123', gen_salt('bf', 10)), true, now())
ON CONFLICT (email) DO NOTHING;

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

-- categories + products
INSERT INTO categories (id, name, created_at) VALUES
  (gen_random_uuid(), 'Bebidas', now()),
  (gen_random_uuid(), 'Mercearia', now())
ON CONFLICT (name) DO NOTHING;

INSERT INTO products (id, category_id, sku, barcode, name, description, unit, cost_price, price_cash, promo_price, min_stock, active, created_at)
SELECT gen_random_uuid(), c.id, 'SKU-COCA-2L', '7890000000000', 'Coca-Cola 2L', '', 'UN', 7.00, 10.90, NULL, 5, true, now()
FROM categories c WHERE c.name='Bebidas'
ON CONFLICT (sku) DO NOTHING;

-- initial stock balance + movement
INSERT INTO inventory_balances (product_id, qty_on_hand, updated_at)
SELECT p.id, 20, now() FROM products p WHERE p.sku='SKU-COCA-2L'
ON CONFLICT (product_id) DO NOTHING;

COMMIT;
