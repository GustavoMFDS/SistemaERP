-- 0021_pdv_operational_permissions.up.sql

BEGIN;

INSERT INTO permissions(id, code, description)
VALUES (gen_random_uuid(), 'sale:discount', 'Aplicar desconto no PDV')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code='sale:discount'
WHERE r.name IN ('admin','manager')
ON CONFLICT DO NOTHING;

COMMIT;
