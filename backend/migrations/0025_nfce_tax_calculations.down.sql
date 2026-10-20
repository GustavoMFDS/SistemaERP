-- 0025_nfce_tax_calculations.down.sql

BEGIN;

DROP TRIGGER IF EXISTS invoice_item_tax_calculations_immutable
  ON invoice_item_tax_calculations;
DROP FUNCTION IF EXISTS prevent_invoice_item_tax_calculation_mutation();
DROP INDEX IF EXISTS invoice_item_tax_calculations_tenant_sale_idx;
DROP TABLE IF EXISTS invoice_item_tax_calculations;

DROP INDEX IF EXISTS sale_item_fiscal_snapshots_tenant_item_sale_unique;
DROP INDEX IF EXISTS invoices_tenant_id_id_sale_id_unique;

COMMIT;
