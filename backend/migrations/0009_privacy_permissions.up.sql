-- 0009_privacy_permissions.up.sql

BEGIN;

INSERT INTO permissions (id, code, description) VALUES
  (gen_random_uuid(), 'privacy:read', 'Consultar requisicoes LGPD e consentimentos'),
  (gen_random_uuid(), 'privacy:write', 'Processar requisicoes LGPD e consentimentos')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code IN ('privacy:read', 'privacy:write')
WHERE r.name='admin'
ON CONFLICT DO NOTHING;

COMMIT;
