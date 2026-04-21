BEGIN;

DROP INDEX IF EXISTS products_active_min_stock_idx;
DROP INDEX IF EXISTS products_active_name_idx;
DROP INDEX IF EXISTS products_name_idx;

COMMIT;
