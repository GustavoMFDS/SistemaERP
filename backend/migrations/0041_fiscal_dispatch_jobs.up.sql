BEGIN;
-- One durable, tenant-scoped fiscal dispatch state per sale. This is a
-- monitoring/preflight queue: no invoice numbers or SEFAZ calls originate here.
CREATE TABLE fiscal_dispatch_jobs (
 tenant_id uuid NOT NULL,
 sale_id uuid NOT NULL,
 state text NOT NULL DEFAULT 'pending'
   CHECK (state IN (
     'pending', 'manual_review', 'waiting_sale', 'cancelled_review',
     'unsupported_model', 'awaiting_configuration', 'awaiting_fiscal_processing',
     'reserved', 'xml_generated', 'signed', 'awaiting_sefaz',
     'authorized', 'missing_processed_xml', 'rejected_review', 'requires_review'
   )),
 check_count bigint NOT NULL DEFAULT 0 CHECK (check_count >= 0),
 last_checked_at timestamptz NULL,
 next_check_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (tenant_id,sale_id),
 CONSTRAINT fiscal_dispatch_intent_fk
  FOREIGN KEY (tenant_id,sale_id)
  REFERENCES sale_fiscal_intents(tenant_id,sale_id) ON DELETE CASCADE
);
CREATE INDEX fiscal_dispatch_due_idx
 ON fiscal_dispatch_jobs(next_check_at,tenant_id,sale_id);
-- Backfill obligations created prior to the scheduler deployment. Legacy
-- sales are never automatically advanced toward issuance.
INSERT INTO fiscal_dispatch_jobs(tenant_id,sale_id,state)
 SELECT tenant_id,sale_id,
   CASE WHEN legacy_review THEN 'manual_review' ELSE 'pending' END
 FROM sale_fiscal_intents;

CREATE FUNCTION enqueue_required_fiscal_dispatch()
RETURNS trigger LANGUAGE plpgsql AS $dispatch$
BEGIN
 INSERT INTO fiscal_dispatch_jobs(tenant_id,sale_id,state)
 VALUES (NEW.tenant_id,NEW.sale_id,
   CASE WHEN NEW.legacy_review THEN 'manual_review' ELSE 'pending' END)
 ON CONFLICT (tenant_id,sale_id) DO NOTHING;
 RETURN NEW;
END
$dispatch$;
CREATE TRIGGER fiscal_intent_enqueue_dispatch
AFTER INSERT ON sale_fiscal_intents FOR EACH ROW
EXECUTE FUNCTION enqueue_required_fiscal_dispatch();
COMMIT;
