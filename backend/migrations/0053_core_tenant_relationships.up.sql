-- 0053: add tenant-safe relationships to the original retail tables.
-- All constraints are validated immediately; no data repair or reassignment.
BEGIN;
CREATE UNIQUE INDEX core53_categories_tenant_id_uidx ON categories(tenant_id, id);
CREATE UNIQUE INDEX core53_customers_tenant_id_uidx ON customers(tenant_id, id);
CREATE UNIQUE INDEX core53_cash_registers_tenant_id_uidx ON cash_registers(tenant_id, id);
ALTER TABLE products ADD CONSTRAINT core53_products_category_id_fk
  FOREIGN KEY (tenant_id, category_id) REFERENCES categories(tenant_id, id) ON DELETE SET NULL (category_id);
ALTER TABLE inventory_balances ADD CONSTRAINT core53_inventory_balances_product_id_fk
  FOREIGN KEY (tenant_id, product_id) REFERENCES products(tenant_id, id) ON DELETE CASCADE;
ALTER TABLE inventory_movements ADD CONSTRAINT core53_inventory_movements_product_id_fk
  FOREIGN KEY (tenant_id, product_id) REFERENCES products(tenant_id, id) ON DELETE RESTRICT;
ALTER TABLE cash_sessions ADD CONSTRAINT core53_cash_sessions_cash_register_id_fk
  FOREIGN KEY (tenant_id, cash_register_id) REFERENCES cash_registers(tenant_id, id) ON DELETE RESTRICT;
ALTER TABLE sales ADD CONSTRAINT core53_sales_cash_session_id_fk
  FOREIGN KEY (tenant_id, cash_session_id) REFERENCES cash_sessions(tenant_id, id) ON DELETE RESTRICT;
ALTER TABLE sales ADD CONSTRAINT core53_sales_customer_id_fk
  FOREIGN KEY (tenant_id, customer_id) REFERENCES customers(tenant_id, id) ON DELETE SET NULL (customer_id);
ALTER TABLE sale_items ADD CONSTRAINT core53_sale_items_sale_id_fk
  FOREIGN KEY (tenant_id, sale_id) REFERENCES sales(tenant_id, id) ON DELETE CASCADE;
ALTER TABLE sale_items ADD CONSTRAINT core53_sale_items_product_id_fk
  FOREIGN KEY (tenant_id, product_id) REFERENCES products(tenant_id, id) ON DELETE RESTRICT;
ALTER TABLE payments ADD CONSTRAINT core53_payments_sale_id_fk
  FOREIGN KEY (tenant_id, sale_id) REFERENCES sales(tenant_id, id) ON DELETE CASCADE;
ALTER TABLE cash_movements ADD CONSTRAINT core53_cash_movements_cash_session_id_fk
  FOREIGN KEY (tenant_id, cash_session_id) REFERENCES cash_sessions(tenant_id, id) ON DELETE RESTRICT;
ALTER TABLE cash_session_reconciliations ADD CONSTRAINT core53_cash_session_reconciliations_cash_session_id_fk
  FOREIGN KEY (tenant_id, cash_session_id) REFERENCES cash_sessions(tenant_id, id) ON DELETE CASCADE;
COMMIT;
