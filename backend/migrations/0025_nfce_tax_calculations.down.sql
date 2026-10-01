-- 0025_nfce_tax_calculations.down.sql

BEGIN;

DROP TRIGGER IF EXISTS invoice_item_tax_calculations_immutable
  ON invoice_item_tax_calculations;
DROP FUNCTION IF EXISTS prevent_invoice_item_tax_calculation_mutation();
DROP INDEX IF EXISTS invoice_item_tax_calculations_tenant_sale_idx;
DROP TABLE IF EXISTS invoice_item_tax_calculations;

-- Keep composite unique indexes. They strengthen tenant/sale referential
-- integrity and are safe for later migrations to reuse.

COMMIT;
