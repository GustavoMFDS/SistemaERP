-- 0027_nfce_cancellation_events.up.sql

BEGIN;

ALTER TABLE invoices
  ADD COLUMN IF NOT EXISTS cancellation_protocol text NULL,
  ADD COLUMN IF NOT EXISTS cancelled_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS cancellation_reason text NULL;

DO $do$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM invoices
    WHERE status='cancelled'
      AND (
        NULLIF(btrim(cancellation_protocol), '') IS NULL
        OR cancelled_at IS NULL
        OR NULLIF(btrim(cancellation_reason), '') IS NULL
      )
  ) THEN
    RAISE EXCEPTION 'existing cancelled invoices must be reconciled before applying migration 0027';
  END IF;
END
$do$;

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_cancellation_fields_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_cancellation_fields_check
  CHECK (
    (
      status='cancelled'
      AND NULLIF(btrim(cancellation_protocol), '') IS NOT NULL
      AND cancelled_at IS NOT NULL
      AND NULLIF(btrim(cancellation_reason), '') IS NOT NULL
    )
    OR (
      status<>'cancelled'
      AND cancellation_protocol IS NULL
      AND cancelled_at IS NULL
      AND cancellation_reason IS NULL
    )
  );

CREATE TABLE invoice_fiscal_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL,
  invoice_id uuid NOT NULL,
  event_type text NOT NULL,
  sequence integer NOT NULL,
  event_id text NOT NULL,
  environment text NOT NULL,
  status text NOT NULL DEFAULT 'prepared',
  justification text NOT NULL,
  signed_xml bytea NULL,
  signed_sha256 text NULL,
  response_xml bytea NULL,
  response_sha256 text NULL,
  status_code integer NULL,
  reason text NULL,
  protocol text NULL,
  registered_at timestamptz NULL,
  created_by_user_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT invoice_fiscal_events_invoice_tenant_fk
    FOREIGN KEY (tenant_id, invoice_id)
    REFERENCES invoices(tenant_id, id)
    ON DELETE RESTRICT,

  CONSTRAINT invoice_fiscal_events_created_by_tenant_fk
    FOREIGN KEY (created_by_user_id, tenant_id)
    REFERENCES user_tenants(user_id, tenant_id)
    ON DELETE RESTRICT,

  CONSTRAINT invoice_fiscal_events_type_check
    CHECK (event_type IN ('110111')),

  CONSTRAINT invoice_fiscal_events_sequence_check
    CHECK (sequence BETWEEN 1 AND 99),

  CONSTRAINT invoice_fiscal_events_event_id_check
    CHECK (
      event_id ~ '^ID110111[0-9]{6}[A-Z0-9]{12}[0-9]{28}$'
    ),

  CONSTRAINT invoice_fiscal_events_environment_check
    CHECK (environment IN ('homologation','production')),

  CONSTRAINT invoice_fiscal_events_status_check
    CHECK (status IN ('prepared','signed','submitted','registered','rejected')),

  CONSTRAINT invoice_fiscal_events_justification_check
    CHECK (char_length(btrim(justification)) BETWEEN 15 AND 255),

  CONSTRAINT invoice_fiscal_events_signed_payload_check
    CHECK (
      status='prepared'
      OR (
        octet_length(signed_xml) > 0
        AND signed_sha256 ~ '^[0-9a-f]{64}$'
      )
    ),

  CONSTRAINT invoice_fiscal_events_response_check
    CHECK (
      status NOT IN ('registered','rejected')
      OR (
        octet_length(response_xml) > 0
        AND response_sha256 ~ '^[0-9a-f]{64}$'
        AND status_code IS NOT NULL
        AND NULLIF(btrim(reason), '') IS NOT NULL
      )
    ),

  CONSTRAINT invoice_fiscal_events_registered_check
    CHECK (
      status<>'registered'
      OR (
        status_code=135
        AND NULLIF(btrim(protocol), '') IS NOT NULL
        AND registered_at IS NOT NULL
      )
    ),

  CONSTRAINT invoice_fiscal_events_rejected_check
    CHECK (
      status<>'rejected'
      OR (
        status_code<>135
        AND protocol IS NULL
        AND registered_at IS NULL
      )
    ),

  CONSTRAINT invoice_fiscal_events_tenant_event_unique
    UNIQUE (tenant_id, event_id),

  CONSTRAINT invoice_fiscal_events_invoice_type_sequence_unique
    UNIQUE (tenant_id, invoice_id, event_type, sequence)
);

CREATE INDEX invoice_fiscal_events_tenant_invoice_created_idx
  ON invoice_fiscal_events(tenant_id, invoice_id, created_at DESC);

CREATE OR REPLACE FUNCTION prevent_invoice_fiscal_event_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'invoice fiscal events cannot be deleted';
END;
$$;

CREATE TRIGGER invoice_fiscal_events_no_delete
BEFORE DELETE ON invoice_fiscal_events
FOR EACH ROW
EXECUTE FUNCTION prevent_invoice_fiscal_event_delete();

COMMIT;
