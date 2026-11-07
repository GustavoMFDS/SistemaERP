-- Production-safe reference seed. This creates RBAC definitions only.
-- It never creates a company, user, password, product, or demo transaction.
-- Create the first administrator with `esterium-bootstrap-admin` after migrations.

BEGIN;

INSERT INTO roles (id, name) VALUES
  (gen_random_uuid(), 'admin'),
  (gen_random_uuid(), 'manager'),
  (gen_random_uuid(), 'cashier')
ON CONFLICT (name) DO NOTHING;

INSERT INTO permissions (id, code, description) VALUES
  (gen_random_uuid(), 'product:read', 'Listar e visualizar produtos'),
  (gen_random_uuid(), 'product:write', 'Criar e editar produtos'),
  (gen_random_uuid(), 'inventory:read', 'Consultar estoque e movimentacoes'),
  (gen_random_uuid(), 'inventory:adjust', 'Ajustar estoque manualmente'),
  (gen_random_uuid(), 'procurement:read', 'Consultar fornecedores e compras'),
  (gen_random_uuid(), 'procurement:write', 'Criar e alterar fornecedores e compras'),
  (gen_random_uuid(), 'procurement:receive', 'Receber compras e atualizar estoque'),
  (gen_random_uuid(), 'cash:open', 'Abrir caixa'),
  (gen_random_uuid(), 'cash:move', 'Registrar sangria e suprimento de caixa'),
  (gen_random_uuid(), 'cash:close', 'Fechar caixa'),
  (gen_random_uuid(), 'sale:read', 'Consultar vendas'),
  (gen_random_uuid(), 'sale:write', 'Criar e finalizar venda'),
  (gen_random_uuid(), 'sale:cancel', 'Cancelar venda'),
  (gen_random_uuid(), 'sale:return', 'Registrar devolucoes e trocas'),
  (gen_random_uuid(), 'sale:discount', 'Aplicar desconto no PDV'),
  (gen_random_uuid(), 'finance:read', 'Consultar financeiro'),
  (gen_random_uuid(), 'finance:reconcile', 'Conciliar pagamentos e reembolsos'),
  (gen_random_uuid(), 'invoice:generate', 'Gerar XML NF-e'),
  (gen_random_uuid(), 'invoice:read', 'Consultar XML NF-e'),
  (gen_random_uuid(), 'privacy:read', 'Consultar requisicoes LGPD e consentimentos'),
  (gen_random_uuid(), 'privacy:write', 'Processar requisicoes LGPD e consentimentos'),
  (gen_random_uuid(), 'audit:read', 'Consultar logs de auditoria')
ON CONFLICT (code) DO NOTHING;

-- Admin receives the complete permission catalogue, including permissions
-- added by later migrations (for example team:manage and return overrides).
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r CROSS JOIN permissions p
WHERE r.name='admin'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r JOIN permissions p ON p.code IN (
  'product:read','product:write','inventory:read','inventory:adjust',
  'procurement:read','procurement:write','procurement:receive',
  'cash:open','cash:move','cash:close','sale:read','sale:write','sale:cancel',
  'sale:return','sale:discount','finance:read','finance:reconcile',
  'invoice:generate','invoice:read','customer:read','customer:write'
)
WHERE r.name='manager'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r JOIN permissions p ON p.code IN (
  'product:read','inventory:read','cash:open','cash:close','sale:read','sale:write'
)
WHERE r.name='cashier'
ON CONFLICT DO NOTHING;

COMMIT;
