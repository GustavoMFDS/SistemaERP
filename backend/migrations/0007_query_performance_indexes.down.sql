-- 0007_query_performance_indexes.down.sql

BEGIN;

DROP INDEX IF EXISTS invoice_xml_files_tenant_invoice_created_idx;
DROP INDEX IF EXISTS ledger_entries_tenant_type_created_idx;
DROP INDEX IF EXISTS idempotency_keys_tenant_operation_key_idx;
DROP INDEX IF EXISTS products_tenant_active_created_idx;
DROP INDEX IF EXISTS sales_tenant_status_created_idx;
DROP INDEX IF EXISTS payments_tenant_sale_idx;
DROP INDEX IF EXISTS sale_items_tenant_product_idx;
DROP INDEX IF EXISTS sale_items_tenant_sale_idx;

COMMIT;
