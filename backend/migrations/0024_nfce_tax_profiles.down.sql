-- 0024_nfce_tax_profiles.down.sql

BEGIN;

DROP INDEX IF EXISTS sale_item_fiscal_snapshots_tenant_sale_idx;
DROP TABLE IF EXISTS sale_item_fiscal_snapshots;
DROP TABLE IF EXISTS product_fiscal_profiles;

DROP INDEX IF EXISTS sale_items_tenant_id_id_unique;
DROP INDEX IF EXISTS products_tenant_id_id_unique;

COMMIT;
