-- 0006_privacy_audit_controls.up.sql

BEGIN;

ALTER TABLE audit_logs
  ADD COLUMN IF NOT EXISTS request_id text NULL,
  ADD COLUMN IF NOT EXISTS resource_type text NULL,
  ADD COLUMN IF NOT EXISTS resource_id uuid NULL;

UPDATE audit_logs
SET resource_type = entity_type,
    resource_id = entity_id
WHERE resource_type IS NULL;

CREATE INDEX IF NOT EXISTS audit_logs_tenant_created_idx ON audit_logs(tenant_id, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_tenant_action_created_idx ON audit_logs(tenant_id, action, created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_request_id_idx ON audit_logs(request_id) WHERE request_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS data_subject_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  subject_type text NOT NULL,
  subject_id uuid NULL,
  requester_email citext NULL,
  request_type text NOT NULL,
  status text NOT NULL DEFAULT 'open',
  notes text NULL,
  requested_at timestamptz NOT NULL DEFAULT now(),
  resolved_at timestamptz NULL,
  created_by_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  request_id text NULL,
  CONSTRAINT dsr_type_check CHECK (request_type IN ('export','correction','anonymization','deletion','blocking')),
  CONSTRAINT dsr_status_check CHECK (status IN ('open','in_review','completed','rejected'))
);

CREATE INDEX IF NOT EXISTS data_subject_requests_tenant_status_idx
  ON data_subject_requests(tenant_id, status, requested_at DESC);

CREATE TABLE IF NOT EXISTS consent_records (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  subject_type text NOT NULL,
  subject_id uuid NULL,
  purpose text NOT NULL,
  consent_text_version text NOT NULL,
  consented_at timestamptz NOT NULL DEFAULT now(),
  source text NOT NULL,
  withdrawn_at timestamptz NULL,
  request_id text NULL
);

CREATE INDEX IF NOT EXISTS consent_records_tenant_subject_idx
  ON consent_records(tenant_id, subject_type, subject_id, purpose, consented_at DESC);

COMMIT;
