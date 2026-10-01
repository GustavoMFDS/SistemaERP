-- 0026_nfce_snapshot_identity.up.sql

BEGIN;

-- Bridge migration for databases that already applied 0024/0025 before product
-- identity became part of the immutable NFC-e item snapshot.
ALTER TABLE sale_item_fiscal_snapshots
  ADD COLUMN IF NOT EXISTS product_code text NULL,
  ADD COLUMN IF NOT EXISTS product_description text NULL,
  ADD COLUMN IF NOT EXISTS unit text NULL;

-- Fiscal transmission is still disabled in production. Existing rows can only
-- be pre-release/homologation reservations, so backfill from the tenant-scoped
-- product record before making the snapshot fields mandatory.
UPDATE sale_item_fiscal_snapshots s
SET product_code = p.sku,
    product_description = p.name,
    unit = p.unit
FROM products p
WHERE p.tenant_id = s.tenant_id
  AND p.id = s.product_id
  AND (
    s.product_code IS NULL
    OR s.product_description IS NULL
    OR s.unit IS NULL
  );

DO $do$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM sale_item_fiscal_snapshots
    WHERE NULLIF(btrim(product_code), '') IS NULL
       OR NULLIF(btrim(product_description), '') IS NULL
       OR NULLIF(btrim(unit), '') IS NULL
  ) THEN
    RAISE EXCEPTION 'cannot backfill immutable NFC-e product identity snapshots';
  END IF;
END
$do$;

ALTER TABLE sale_item_fiscal_snapshots
  ALTER COLUMN product_code SET NOT NULL,
  ALTER COLUMN product_description SET NOT NULL,
  ALTER COLUMN unit SET NOT NULL;

ALTER TABLE sale_item_fiscal_snapshots
  DROP CONSTRAINT IF EXISTS sale_item_fiscal_snapshots_product_code_check;
ALTER TABLE sale_item_fiscal_snapshots
  ADD CONSTRAINT sale_item_fiscal_snapshots_product_code_check
  CHECK (NULLIF(btrim(product_code), '') IS NOT NULL AND char_length(product_code) <= 60);

ALTER TABLE sale_item_fiscal_snapshots
  DROP CONSTRAINT IF EXISTS sale_item_fiscal_snapshots_product_description_check;
ALTER TABLE sale_item_fiscal_snapshots
  ADD CONSTRAINT sale_item_fiscal_snapshots_product_description_check
  CHECK (
    NULLIF(btrim(product_description), '') IS NOT NULL
    AND char_length(product_description) <= 120
  );

ALTER TABLE sale_item_fiscal_snapshots
  DROP CONSTRAINT IF EXISTS sale_item_fiscal_snapshots_unit_check;
ALTER TABLE sale_item_fiscal_snapshots
  ADD CONSTRAINT sale_item_fiscal_snapshots_unit_check
  CHECK (NULLIF(btrim(unit), '') IS NOT NULL AND char_length(unit) <= 6);

CREATE OR REPLACE FUNCTION prevent_sale_item_fiscal_snapshot_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'sale item fiscal snapshots are immutable';
END;
$$;

DROP TRIGGER IF EXISTS invoice_item_tax_calculations_immutable
  ON invoice_item_tax_calculations;

CREATE TRIGGER invoice_item_tax_calculations_immutable
BEFORE UPDATE ON invoice_item_tax_calculations
FOR EACH ROW
EXECUTE FUNCTION prevent_invoice_item_tax_calculation_mutation();

DROP TRIGGER IF EXISTS sale_item_fiscal_snapshots_immutable
  ON sale_item_fiscal_snapshots;

CREATE TRIGGER sale_item_fiscal_snapshots_immutable
BEFORE UPDATE ON sale_item_fiscal_snapshots
FOR EACH ROW
EXECUTE FUNCTION prevent_sale_item_fiscal_snapshot_mutation();

COMMIT;
