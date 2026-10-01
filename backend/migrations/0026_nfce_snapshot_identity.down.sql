-- 0026_nfce_snapshot_identity.down.sql

BEGIN;

DROP TRIGGER IF EXISTS invoice_item_tax_calculations_immutable
  ON invoice_item_tax_calculations;

CREATE TRIGGER invoice_item_tax_calculations_immutable
BEFORE UPDATE OR DELETE ON invoice_item_tax_calculations
FOR EACH ROW
EXECUTE FUNCTION prevent_invoice_item_tax_calculation_mutation();

DROP TRIGGER IF EXISTS sale_item_fiscal_snapshots_immutable
  ON sale_item_fiscal_snapshots;
DROP FUNCTION IF EXISTS prevent_sale_item_fiscal_snapshot_mutation();

ALTER TABLE sale_item_fiscal_snapshots
  DROP CONSTRAINT IF EXISTS sale_item_fiscal_snapshots_unit_check,
  DROP CONSTRAINT IF EXISTS sale_item_fiscal_snapshots_product_description_check,
  DROP CONSTRAINT IF EXISTS sale_item_fiscal_snapshots_product_code_check,
  DROP COLUMN IF EXISTS unit,
  DROP COLUMN IF EXISTS product_description,
  DROP COLUMN IF EXISTS product_code;

COMMIT;
