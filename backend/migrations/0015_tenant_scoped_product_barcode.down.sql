-- 0015_tenant_scoped_product_barcode.down.sql

BEGIN;

DROP INDEX IF EXISTS products_tenant_barcode_unique;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM products
    WHERE barcode IS NOT NULL
    GROUP BY barcode
    HAVING count(*) > 1
  ) THEN
    RAISE EXCEPTION 'cannot restore global barcode uniqueness: duplicate barcodes exist across tenants';
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS products_barcode_unique
  ON products(barcode)
  WHERE barcode IS NOT NULL;

COMMIT;
