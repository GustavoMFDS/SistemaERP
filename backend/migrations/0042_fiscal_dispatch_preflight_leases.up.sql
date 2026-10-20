BEGIN;
-- No remote fiscal operation is allowed by this schema. The lease only
-- fences concurrent attempts at local, fail-closed fiscal preflight.
ALTER TABLE fiscal_dispatch_jobs
 ADD COLUMN lease_token uuid NULL,
 ADD COLUMN lease_owner text NULL,
 ADD COLUMN lease_until timestamptz NULL,
 ADD COLUMN claim_count bigint NOT NULL DEFAULT 0
   CHECK (claim_count >= 0),
 ADD COLUMN last_claimed_at timestamptz NULL,
 ADD COLUMN last_result_code text NULL,
 ADD COLUMN next_claim_at timestamptz NOT NULL DEFAULT now(),
 ADD CONSTRAINT fiscal_dispatch_lease_consistency CHECK (
   (lease_token IS NULL AND lease_owner IS NULL AND lease_until IS NULL)
   OR
   (lease_token IS NOT NULL AND lease_owner IS NOT NULL AND
     length(btrim(lease_owner)) BETWEEN 1 AND 64 AND lease_until IS NOT NULL)
 ),
 ADD CONSTRAINT fiscal_dispatch_result_code_check CHECK (
   last_result_code IS NULL OR last_result_code IN (
     'inputs_present_unverified', 'issuer_incomplete',
     'product_classification_missing', 'payment_review_required', 'internal_error'
   )
 );
CREATE INDEX fiscal_dispatch_claim_due_idx
 ON fiscal_dispatch_jobs(next_claim_at,tenant_id,sale_id)
 WHERE state='awaiting_fiscal_processing';
COMMIT;
