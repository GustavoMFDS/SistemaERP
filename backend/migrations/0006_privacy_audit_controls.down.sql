-- 0006_privacy_audit_controls.down.sql

BEGIN;

DROP TABLE IF EXISTS consent_records;
DROP TABLE IF EXISTS data_subject_requests;

DROP INDEX IF EXISTS audit_logs_request_id_idx;
DROP INDEX IF EXISTS audit_logs_tenant_action_created_idx;
DROP INDEX IF EXISTS audit_logs_tenant_created_idx;

ALTER TABLE audit_logs
  DROP COLUMN IF EXISTS resource_id,
  DROP COLUMN IF EXISTS resource_type,
  DROP COLUMN IF EXISTS request_id;

COMMIT;
