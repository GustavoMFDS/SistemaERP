BEGIN;
-- A durable consultation-only recovery queue for ambiguous submitted NFC-e.
-- It must NEVER be used to authorize/resend XML or manufacture an SEFAZ result.
CREATE TABLE nfce_consult_recovery_jobs (
  tenant_id uuid NOT NULL,
  invoice_id uuid NOT NULL,
  access_key text NOT NULL CHECK (
    access_key ~ '^[0-9]{6}[A-Z0-9]{12}[0-9]{26}$'
  ),
  environment text NOT NULL CHECK (environment='homologation'),
  state text NOT NULL DEFAULT 'awaiting_consultation'
    CHECK (state IN ('awaiting_consultation','manual_review','resolved')),
  attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count BETWEEN 0 AND 10),
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  lease_token uuid,
  lease_owner text,
  lease_until timestamptz,
  last_result_code text CHECK (
    last_result_code IS NULL OR last_result_code IN (
      'not_found','pending','transport_error','invalid_protocol','manual_review'
    )
  ),
  last_status_code integer,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,invoice_id),
  CONSTRAINT nfce_consult_recovery_invoice_fk FOREIGN KEY (tenant_id, invoice_id)
    REFERENCES invoices(tenant_id,id) ON DELETE RESTRICT,
  CONSTRAINT nfce_consult_recovery_lease_shape_check CHECK (
    (lease_token IS NULL AND lease_owner IS NULL AND lease_until IS NULL)
    OR (lease_token IS NOT NULL AND NULLIF(btrim(lease_owner),'') IS NOT NULL AND lease_until IS NOT NULL)
  )
);
CREATE UNIQUE INDEX nfce_consult_recovery_lease_token_unique
  ON nfce_consult_recovery_jobs(lease_token) WHERE lease_token IS NOT NULL;
CREATE INDEX nfce_consult_recovery_due_idx
  ON nfce_consult_recovery_jobs(next_attempt_at,tenant_id,invoice_id)
  WHERE state='awaiting_consultation';
COMMIT;
