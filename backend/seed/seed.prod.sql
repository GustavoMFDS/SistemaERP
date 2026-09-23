-- Production-safe seed placeholder.
-- This file intentionally does not create default users or passwords.
--
-- Create the first production admin through a controlled bootstrap process:
-- 1. Generate a strong temporary password outside source control.
-- 2. Generate a bcrypt hash with an approved operational tool.
-- 3. Insert the user, tenant mapping, admin role, and force an immediate password change.
--
-- See README.md for the operational checklist.

BEGIN;

INSERT INTO permissions (id, code, description) VALUES
  (gen_random_uuid(), 'privacy:read', 'Consultar requisicoes LGPD e consentimentos'),
  (gen_random_uuid(), 'privacy:write', 'Processar requisicoes LGPD e consentimentos'),
  (gen_random_uuid(), 'audit:read', 'Consultar logs de auditoria'),
  (gen_random_uuid(), 'cash:move', 'Registrar sangria e suprimento de caixa')
ON CONFLICT (code) DO NOTHING;

COMMIT;
