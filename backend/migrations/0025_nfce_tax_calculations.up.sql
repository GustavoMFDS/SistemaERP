-- 0025_nfce_tax_calculations.up.sql

BEGIN;

-- Composite keys bind every calculated item to the same tenant/sale as the
-- reserved invoice and the immutable classification snapshot.
CREATE UNIQUE INDEX IF NOT EXISTS invoices_tenant_id_id_sale_id_unique
  ON invoices(tenant_id, id, sale_id);

CREATE UNIQUE INDEX IF NOT EXISTS sale_item_fiscal_snapshots_tenant_item_sale_unique
  ON sale_item_fiscal_snapshots(tenant_id, sale_item_id, sale_id);

CREATE TABLE IF NOT EXISTS invoice_item_tax_calculations (
  tenant_id uuid NOT NULL,
  invoice_id uuid NOT NULL,
  sale_id uuid NOT NULL,
  sale_item_id uuid NOT NULL,

  calculation_version text NOT NULL,
  legacy_tax jsonb NOT NULL DEFAULT '{}'::jsonb,
  rtc_tax jsonb NOT NULL DEFAULT '{}'::jsonb,
  calculation_sha256 text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),

  PRIMARY KEY (tenant_id, invoice_id, sale_item_id),

  CONSTRAINT invoice_item_tax_calculations_invoice_fk
    FOREIGN KEY (tenant_id, invoice_id, sale_id)
    REFERENCES invoices(tenant_id, id, sale_id)
    ON DELETE CASCADE,

  CONSTRAINT invoice_item_tax_calculations_snapshot_fk
    FOREIGN KEY (tenant_id, sale_item_id, sale_id)
    REFERENCES sale_item_fiscal_snapshots(tenant_id, sale_item_id, sale_id)
    ON DELETE RESTRICT,

  CONSTRAINT invoice_item_tax_calculations_version_check
    CHECK (NULLIF(btrim(calculation_version), '') IS NOT NULL),

  CONSTRAINT invoice_item_tax_calculations_legacy_object_check
    CHECK (jsonb_typeof(legacy_tax) = 'object'),

  CONSTRAINT invoice_item_tax_calculations_rtc_object_check
    CHECK (jsonb_typeof(rtc_tax) = 'object'),

  CONSTRAINT invoice_item_tax_calculations_sha256_check
    CHECK (calculation_sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX IF NOT EXISTS invoice_item_tax_calculations_tenant_sale_idx
  ON invoice_item_tax_calculations(tenant_id, sale_id);

CREATE OR REPLACE FUNCTION prevent_invoice_item_tax_calculation_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'invoice item tax calculations are immutable';
END;
$$;

DROP TRIGGER IF EXISTS invoice_item_tax_calculations_immutable
  ON invoice_item_tax_calculations;

CREATE TRIGGER invoice_item_tax_calculations_immutable
BEFORE UPDATE OR DELETE ON invoice_item_tax_calculations
FOR EACH ROW
EXECUTE FUNCTION prevent_invoice_item_tax_calculation_mutation();

COMMIT;
