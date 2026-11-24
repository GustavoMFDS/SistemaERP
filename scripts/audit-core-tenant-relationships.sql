-- Read-only diagnostics for a verified backup or authorized database connection.
-- No row identifiers or business data are emitted; investigate nonzero counts
-- before upgrading. The migration itself validates under DDL locks.
SELECT relation, mismatches
FROM (
  SELECT 'products.category_id' AS relation, count(*) AS mismatches
  FROM products child
  LEFT JOIN categories parent ON parent.id=child.category_id AND parent.tenant_id=child.tenant_id
  WHERE child.category_id IS NOT NULL AND parent.id IS NULL
  UNION ALL
  SELECT 'inventory_balances.product_id' AS relation, count(*) AS mismatches
  FROM inventory_balances child
  LEFT JOIN products parent ON parent.id=child.product_id AND parent.tenant_id=child.tenant_id
  WHERE child.product_id IS NOT NULL AND parent.id IS NULL
  UNION ALL
  SELECT 'inventory_movements.product_id' AS relation, count(*) AS mismatches
  FROM inventory_movements child
  LEFT JOIN products parent ON parent.id=child.product_id AND parent.tenant_id=child.tenant_id
  WHERE child.product_id IS NOT NULL AND parent.id IS NULL
  UNION ALL
  SELECT 'cash_sessions.cash_register_id' AS relation, count(*) AS mismatches
  FROM cash_sessions child
  LEFT JOIN cash_registers parent ON parent.id=child.cash_register_id AND parent.tenant_id=child.tenant_id
  WHERE child.cash_register_id IS NOT NULL AND parent.id IS NULL
  UNION ALL
  SELECT 'sales.cash_session_id' AS relation, count(*) AS mismatches
  FROM sales child
  LEFT JOIN cash_sessions parent ON parent.id=child.cash_session_id AND parent.tenant_id=child.tenant_id
  WHERE child.cash_session_id IS NOT NULL AND parent.id IS NULL
  UNION ALL
  SELECT 'sales.customer_id' AS relation, count(*) AS mismatches
  FROM sales child
  LEFT JOIN customers parent ON parent.id=child.customer_id AND parent.tenant_id=child.tenant_id
  WHERE child.customer_id IS NOT NULL AND parent.id IS NULL
  UNION ALL
  SELECT 'sale_items.sale_id' AS relation, count(*) AS mismatches
  FROM sale_items child
  LEFT JOIN sales parent ON parent.id=child.sale_id AND parent.tenant_id=child.tenant_id
  WHERE child.sale_id IS NOT NULL AND parent.id IS NULL
  UNION ALL
  SELECT 'sale_items.product_id' AS relation, count(*) AS mismatches
  FROM sale_items child
  LEFT JOIN products parent ON parent.id=child.product_id AND parent.tenant_id=child.tenant_id
  WHERE child.product_id IS NOT NULL AND parent.id IS NULL
  UNION ALL
  SELECT 'payments.sale_id' AS relation, count(*) AS mismatches
  FROM payments child
  LEFT JOIN sales parent ON parent.id=child.sale_id AND parent.tenant_id=child.tenant_id
  WHERE child.sale_id IS NOT NULL AND parent.id IS NULL
  UNION ALL
  SELECT 'cash_movements.cash_session_id' AS relation, count(*) AS mismatches
  FROM cash_movements child
  LEFT JOIN cash_sessions parent ON parent.id=child.cash_session_id AND parent.tenant_id=child.tenant_id
  WHERE child.cash_session_id IS NOT NULL AND parent.id IS NULL
  UNION ALL
  SELECT 'cash_session_reconciliations.cash_session_id' AS relation, count(*) AS mismatches
  FROM cash_session_reconciliations child
  LEFT JOIN cash_sessions parent ON parent.id=child.cash_session_id AND parent.tenant_id=child.tenant_id
  WHERE child.cash_session_id IS NOT NULL AND parent.id IS NULL
) checks
ORDER BY relation;
