BEGIN;
-- Explicitly gated issuance queue. It does not change the sale, tax or invoice
-- contract, and is never active without operator/deployment opt-in.
CREATE TABLE nfce_auto_homologation_jobs (
  tenant_id uuid NOT NULL,
  sale_id uuid NOT NULL,
  state text NOT NULL DEFAULT 'queued'
    CHECK (state IN ('queued','awaiting_sefaz','authorized','manual_review')),
  attempt_count integer NOT NULL DEFAULT 0
    CHECK (attempt_count BETWEEN 0 AND 12),
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  lease_token uuid NULL,
  lease_owner text NULL,
  lease_until timestamptz NULL,
  last_step text NULL CHECK (
    last_step IS NULL OR last_step IN (
      'queued','reserve','sign','authorize','awaiting_sefaz',
      'authorized','manual_review','retry'
    )
  ),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id,sale_id),
  CONSTRAINT nfce_auto_homologation_sale_fk
    FOREIGN KEY (tenant_id,sale_id)
    REFERENCES sale_fiscal_intents(tenant_id,sale_id) ON DELETE RESTRICT,
  CONSTRAINT nfce_auto_homologation_lease_check CHECK (
    (lease_token IS NULL AND lease_owner IS NULL AND lease_until IS NULL)
    OR (lease_token IS NOT NULL AND NULLIF(btrim(lease_owner),'') IS NOT NULL
        AND lease_until IS NOT NULL)
  )
);
CREATE UNIQUE INDEX nfce_auto_homologation_token_unique
  ON nfce_auto_homologation_jobs(lease_token) WHERE lease_token IS NOT NULL;
CREATE INDEX nfce_auto_homologation_due
  ON nfce_auto_homologation_jobs(next_attempt_at,tenant_id,sale_id)
  WHERE state='queued';
COMMIT;
