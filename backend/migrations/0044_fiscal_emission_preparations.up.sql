BEGIN;
-- The emission preparation is a versioned, immutable diagnostic record.
-- No reservation number or signed/authorized invoice is created here.
CREATE TABLE fiscal_emission_preparations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 tenant_id uuid NOT NULL,
 sale_id uuid NOT NULL,
 worker_identity_id uuid NOT NULL REFERENCES fiscal_worker_identities(id) ON DELETE RESTRICT,
 lease_token uuid NOT NULL UNIQUE,
 source_sha256 text NOT NULL CHECK(source_sha256 ~ '^[0-9a-f]{64}$'),
 status text NOT NULL CHECK(status IN ('blocked','tax_review_required')),
 result_code text NOT NULL CHECK(result_code IN (
  'inputs_present_unverified','issuer_incomplete',
  'product_classification_missing','payment_review_required','internal_error'
 )),
 prepared_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT fiscal_emission_preparations_job_fk FOREIGN KEY(tenant_id,sale_id)
  REFERENCES fiscal_dispatch_jobs(tenant_id,sale_id) ON DELETE RESTRICT,
 CONSTRAINT fiscal_emission_preparations_status_result_check CHECK(
  (status='tax_review_required' AND result_code='inputs_present_unverified')
   OR (status='blocked' AND result_code<>'inputs_present_unverified')
 )
);
CREATE INDEX fiscal_emission_preparations_latest_idx
 ON fiscal_emission_preparations(tenant_id,sale_id,prepared_at DESC,id DESC);
CREATE FUNCTION prevent_fiscal_emission_preparation_mutation()
RETURNS trigger LANGUAGE plpgsql AS $immutable$
BEGIN
 RAISE EXCEPTION 'fiscal emission preparation snapshots are immutable';
END
$immutable$;
CREATE TRIGGER fiscal_emission_preparations_immutable
 BEFORE UPDATE OR DELETE ON fiscal_emission_preparations
 FOR EACH ROW EXECUTE FUNCTION prevent_fiscal_emission_preparation_mutation();
COMMIT;
