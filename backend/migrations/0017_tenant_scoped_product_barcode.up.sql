-- 0017_tenant_scoped_product_barcode.up.sql

BEGIN;

-- Barcodes identify a product inside one independent store/company (tenant).
-- Different stores may legitimately carry the same manufacturer GTIN/EAN.
DROP INDEX IF EXISTS products_barcode_unique;

CREATE UNIQUE INDEX IF NOT EXISTS products_tenant_barcode_unique
  ON products(tenant_id, barcode)
  WHERE barcode IS NOT NULL;

COMMIT;
