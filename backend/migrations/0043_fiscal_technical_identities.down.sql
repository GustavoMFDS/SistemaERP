BEGIN;
DO $guard$
BEGIN
 IF EXISTS(SELECT 1 FROM fiscal_preflight_audit_events LIMIT 1)
    OR EXISTS(SELECT 1 FROM fiscal_worker_identities LIMIT 1) THEN
  RAISE EXCEPTION 'cannot discard technical fiscal principal or preflight audit history';
 END IF;
END
$guard$;
DROP TRIGGER fiscal_preflight_lease_audit ON fiscal_dispatch_jobs;
DROP FUNCTION audit_fiscal_preflight_lease_change();
DROP TRIGGER fiscal_preflight_audit_immutable ON fiscal_preflight_audit_events;
DROP FUNCTION prevent_fiscal_preflight_audit_mutation();
DROP TABLE fiscal_preflight_audit_events;
DROP TABLE fiscal_worker_identities;
COMMIT;
