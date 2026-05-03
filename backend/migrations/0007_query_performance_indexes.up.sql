-- 0007_query_performance_indexes.up.sql

BEGIN;

CREATE INDEX IF NOT EXISTS sale_items_tenant_sale_idx ON sale_items(tenant_id, sale_id);
CREATE INDEX IF NOT EXISTS sale_items_tenant_product_idx ON sale_items(tenant_id, product_id);
CREATE INDEX IF NOT EXISTS payments_tenant_sale_idx ON payments(tenant_id, sale_id);
CREATE INDEX IF NOT EXISTS sales_tenant_status_created_idx ON sales(tenant_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS products_tenant_active_created_idx ON products(tenant_id, active, created_at DESC);
CREATE INDEX IF NOT EXISTS idempotency_keys_tenant_operation_key_idx ON idempotency_keys(tenant_id, operation, idem_key);
CREATE INDEX IF NOT EXISTS ledger_entries_tenant_type_created_idx ON ledger_entries(tenant_id, entry_type, created_at DESC);
CREATE INDEX IF NOT EXISTS invoice_xml_files_tenant_invoice_created_idx ON invoice_xml_files(tenant_id, invoice_id, created_at DESC);

COMMIT;
