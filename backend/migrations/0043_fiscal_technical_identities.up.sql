BEGIN;
-- Machine principals are distinct from human users. No synthetic user is
-- inserted into users or user_tenants, nor attributed in audit_logs.
CREATE TABLE fiscal_worker_identities (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 owner_name text NOT NULL UNIQUE
  CHECK (length(btrim(owner_name)) BETWEEN 1 AND 64),
 kind text NOT NULL DEFAULT 'local_preflight'
  CHECK (kind='local_preflight'),
 registered_at timestamptz NOT NULL DEFAULT now(),
 renewed_at timestamptz NOT NULL DEFAULT now(),
 active_until timestamptz NOT NULL,
 revoked_at timestamptz NULL,
 CONSTRAINT worker_identity_lifetime CHECK(active_until>registered_at)
);
-- Append-only chain. The operational lease row may be released/reclaimed;
-- its immutable evidence must survive ordinary processing and restarts.
CREATE TABLE fiscal_preflight_audit_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 tenant_id uuid NOT NULL,
 sale_id uuid NOT NULL,
 lease_token uuid NOT NULL,
 worker_identity_id uuid NOT NULL
   REFERENCES fiscal_worker_identities(id) ON DELETE RESTRICT,
 event_kind text NOT NULL CHECK(event_kind IN ('claimed','completed')),
 reclaimed_from_token uuid NULL,
 result_code text NULL,
 recorded_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT fiscal_preflight_audit_job_fk
   FOREIGN KEY (tenant_id,sale_id)
   REFERENCES fiscal_dispatch_jobs(tenant_id,sale_id) ON DELETE RESTRICT,
 CONSTRAINT fiscal_preflight_audit_unique_event UNIQUE(lease_token,event_kind),
 CONSTRAINT fiscal_preflight_audit_shape CHECK (
  (event_kind='claimed' AND result_code IS NULL)
  OR (event_kind='completed' AND result_code IS NOT NULL)
 )
);
CREATE INDEX fiscal_preflight_audit_tenant_sale_idx
  ON fiscal_preflight_audit_events(tenant_id,sale_id,id DESC);
CREATE FUNCTION prevent_fiscal_preflight_audit_mutation()
RETURNS trigger LANGUAGE plpgsql AS $immut$
BEGIN
 RAISE EXCEPTION 'fiscal preflight audit events are immutable';
END
$immut$;
CREATE TRIGGER fiscal_preflight_audit_immutable
BEFORE UPDATE OR DELETE ON fiscal_preflight_audit_events
FOR EACH ROW EXECUTE FUNCTION prevent_fiscal_preflight_audit_mutation();

-- Logging is in the SAME transaction as the lease mutation. A missing,
-- disabled or expired principal fails the mutation instead of skipping audit.
CREATE FUNCTION audit_fiscal_preflight_lease_change()
RETURNS trigger LANGUAGE plpgsql AS $audit$
DECLARE principal uuid;
BEGIN
 IF NEW.lease_token IS DISTINCT FROM OLD.lease_token AND NEW.lease_token IS NOT NULL THEN
   SELECT id INTO principal
   FROM fiscal_worker_identities
   WHERE owner_name=NEW.lease_owner
     AND revoked_at IS NULL AND active_until>now();
   IF principal IS NULL THEN RAISE EXCEPTION 'unknown or expired fiscal worker identity'; END IF;
   INSERT INTO fiscal_preflight_audit_events(
     tenant_id,sale_id,lease_token,worker_identity_id,event_kind,reclaimed_from_token
   ) VALUES(NEW.tenant_id,NEW.sale_id,NEW.lease_token,principal,'claimed',OLD.lease_token);
 ELSIF OLD.lease_token IS NOT NULL AND NEW.lease_token IS NULL THEN
   SELECT id INTO principal FROM fiscal_worker_identities
   WHERE owner_name=OLD.lease_owner AND revoked_at IS NULL AND active_until>now();
   IF principal IS NULL THEN RAISE EXCEPTION 'unknown or expired fiscal worker identity'; END IF;
   INSERT INTO fiscal_preflight_audit_events(
     tenant_id,sale_id,lease_token,worker_identity_id,event_kind,result_code
   ) VALUES(OLD.tenant_id,OLD.sale_id,OLD.lease_token,principal,'completed',NEW.last_result_code);
 END IF;
 RETURN NEW;
END
$audit$;
CREATE TRIGGER fiscal_preflight_lease_audit
AFTER UPDATE OF lease_token ON fiscal_dispatch_jobs
FOR EACH ROW EXECUTE FUNCTION audit_fiscal_preflight_lease_change();
COMMIT;
