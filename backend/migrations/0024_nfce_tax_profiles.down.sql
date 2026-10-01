-- 0024_nfce_tax_profiles.down.sql

BEGIN;

DROP INDEX IF EXISTS sale_item_fiscal_snapshots_tenant_sale_idx;
DROP TABLE IF EXISTS sale_item_fiscal_snapshots;
DROP TABLE IF EXISTS product_fiscal_profiles;

-- Keep the composite indexes: they are generally useful and may be referenced
-- by later tenant-isolation migrations. They are safe to retain on rollback.

COMMIT;
