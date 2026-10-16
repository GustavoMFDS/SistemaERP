BEGIN;
-- Every committed server-side sale automatically creates a durable fiscal
-- obligation in the same PostgreSQL transaction, even when the client is
-- offline/retrying and even when the SEFAZ/certificate is not configured.
-- This is NOT an issued or authorized fiscal document.
CREATE TABLE sale_fiscal_intents (
  tenant_id uuid NOT NULL,
  sale_id uuid NOT NULL,
  document_kind text NOT NULL DEFAULT 'nfce'
    CHECK (document_kind IN ('nfce', 'nfe')),
  legacy_review boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, sale_id),
  CONSTRAINT sale_fiscal_intents_sale_fk
    FOREIGN KEY (tenant_id, sale_id)
    REFERENCES sales(tenant_id, id) ON DELETE CASCADE
);

-- Historical transactions need manual reconciliation; never label them as
-- newly emitted invoices or infer fiscal authorization from their sale state.
INSERT INTO sale_fiscal_intents (tenant_id, sale_id, document_kind, legacy_review)
SELECT tenant_id, id, 'nfce', true FROM sales;

CREATE FUNCTION record_required_sale_fiscal_intent()
RETURNS trigger LANGUAGE plpgsql AS $fiscal$
BEGIN
  INSERT INTO sale_fiscal_intents(tenant_id, sale_id, document_kind)
  VALUES (NEW.tenant_id, NEW.id, 'nfce');
  RETURN NEW;
END
$fiscal$;

CREATE TRIGGER sales_require_fiscal_intent
AFTER INSERT ON sales FOR EACH ROW
EXECUTE FUNCTION record_required_sale_fiscal_intent();

CREATE INDEX sale_fiscal_intents_created_idx
ON sale_fiscal_intents(tenant_id, created_at DESC);
COMMIT;
