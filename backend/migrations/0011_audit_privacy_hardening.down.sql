-- 0011_audit_privacy_hardening.down.sql

BEGIN;

DROP INDEX IF EXISTS consent_records_tenant_consented_id_idx;
DROP INDEX IF EXISTS data_subject_requests_tenant_requested_id_idx;
DROP INDEX IF EXISTS audit_logs_tenant_outcome_created_idx;
DROP INDEX IF EXISTS audit_logs_tenant_actor_created_idx;
DROP INDEX IF EXISTS audit_logs_tenant_resource_created_idx;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.code='audit:read';

DELETE FROM permissions
WHERE code='audit:read';

UPDATE data_subject_requests
SET status='in_review'
WHERE status='in_progress';

ALTER TABLE data_subject_requests
  DROP CONSTRAINT IF EXISTS dsr_status_check,
  ADD CONSTRAINT dsr_status_check CHECK (status IN ('open','in_review','completed','rejected'));

COMMIT;
