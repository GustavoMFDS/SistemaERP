-- 0011_audit_privacy_hardening.up.sql

BEGIN;

UPDATE data_subject_requests
SET status='in_progress'
WHERE status='in_review';

ALTER TABLE data_subject_requests
  DROP CONSTRAINT IF EXISTS dsr_status_check,
  ADD CONSTRAINT dsr_status_check CHECK (status IN ('open','in_progress','completed','rejected','cancelled'));

INSERT INTO permissions (id, code, description) VALUES
  (gen_random_uuid(), 'audit:read', 'Consultar logs de auditoria')
ON CONFLICT (code) DO NOTHING;

INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code='audit:read'
WHERE r.name='admin'
ON CONFLICT DO NOTHING;

CREATE INDEX IF NOT EXISTS audit_logs_tenant_resource_created_idx
  ON audit_logs(tenant_id, resource_type, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_tenant_actor_created_idx
  ON audit_logs(tenant_id, actor_user_id, created_at DESC)
  WHERE actor_user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS audit_logs_tenant_outcome_created_idx
  ON audit_logs(tenant_id, (metadata->>'outcome'), created_at DESC);
CREATE INDEX IF NOT EXISTS data_subject_requests_tenant_requested_id_idx
  ON data_subject_requests(tenant_id, requested_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS consent_records_tenant_consented_id_idx
  ON consent_records(tenant_id, consented_at DESC, id DESC);

COMMIT;
