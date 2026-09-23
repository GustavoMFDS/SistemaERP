-- 0017_sales_returns_refunds.down.sql

BEGIN;

DROP TABLE IF EXISTS sales_return_idempotency_keys;
DROP TABLE IF EXISTS sale_refunds;
DROP TABLE IF EXISTS sale_return_items;
DROP TABLE IF EXISTS sale_returns;

COMMIT;
