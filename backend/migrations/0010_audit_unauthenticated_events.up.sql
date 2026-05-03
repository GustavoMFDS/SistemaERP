-- 0010_audit_unauthenticated_events.up.sql

BEGIN;

ALTER TABLE audit_logs
  ALTER COLUMN tenant_id DROP NOT NULL;

COMMIT;
