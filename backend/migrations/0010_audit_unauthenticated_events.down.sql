-- 0010_audit_unauthenticated_events.down.sql

BEGIN;

ALTER TABLE audit_logs
  ALTER COLUMN tenant_id SET NOT NULL;

COMMIT;
