-- CI only: transaction-scoped fixtures on a dedicated migrated PostgreSQL.
-- Never run against a store database. Expect each specific FK to reject.
BEGIN;
DO $test$
DECLARE
  a uuid; b uuid; actor uuid; cat_a uuid; cat_b uuid;
  p_a uuid; p_b uuid; customer_a uuid; customer_b uuid;
  reg_a uuid; reg_b uuid; session_a uuid; session_b uuid;
  sale_a uuid; sale_b uuid; item_a uuid; payment_a uuid;
  movement_a uuid; balance_a uuid; cash_move_a uuid;
  actual text;
BEGIN
  IF current_database() <> 'sistemaemgo_core53_ci' THEN
    RAISE EXCEPTION 'requires isolated core53 CI database';
  END IF;
  INSERT INTO companies(legal_name,trade_name,cnpj) VALUES ('CI Core A','CI Core A','00000000005301') RETURNING id INTO a;
  INSERT INTO companies(legal_name,trade_name,cnpj) VALUES ('CI Core B','CI Core B','00000000005302') RETURNING id INTO b;
  SELECT id INTO STRICT actor FROM users WHERE email='admin@sistema.local';
  INSERT INTO user_tenants(user_id,tenant_id) VALUES (actor,a),(actor,b) ON CONFLICT DO NOTHING;
  INSERT INTO categories(tenant_id,name) VALUES(a,'Core A') RETURNING id INTO cat_a;
  INSERT INTO categories(tenant_id,name) VALUES(b,'Core B') RETURNING id INTO cat_b;
  INSERT INTO products(tenant_id,category_id,sku,name,unit,price_cash)
    VALUES(a,cat_a,'CORE53-A','Core A','UN',10) RETURNING id INTO p_a;
  INSERT INTO products(tenant_id,category_id,sku,name,unit,price_cash)
    VALUES(b,cat_b,'CORE53-B','Core B','UN',10) RETURNING id INTO p_b;
  INSERT INTO customers(tenant_id,name) VALUES(a,'Core A') RETURNING id INTO customer_a;
  INSERT INTO customers(tenant_id,name) VALUES(b,'Core B') RETURNING id INTO customer_b;
  INSERT INTO cash_registers(tenant_id,name) VALUES(a,'Core A') RETURNING id INTO reg_a;
  INSERT INTO cash_registers(tenant_id,name) VALUES(b,'Core B') RETURNING id INTO reg_b;
  INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,status)
    VALUES(a,reg_a,actor,'closed') RETURNING id INTO session_a;
  INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,status)
    VALUES(b,reg_b,actor,'closed') RETURNING id INTO session_b;
  INSERT INTO sales(tenant_id,cash_session_id,customer_id,subtotal,total,created_by_user_id)
    VALUES(a,session_a,customer_a,10,10,actor) RETURNING id INTO sale_a;
  INSERT INTO sales(tenant_id,cash_session_id,customer_id,subtotal,total,created_by_user_id)
    VALUES(b,session_b,customer_b,10,10,actor) RETURNING id INTO sale_b;
  INSERT INTO sale_items(tenant_id,sale_id,product_id,qty,unit_price,subtotal)
    VALUES(a,sale_a,p_a,1,10,10) RETURNING id INTO item_a;
  INSERT INTO payments(tenant_id,sale_id,method,amount)
    VALUES(a,sale_a,'cash',10) RETURNING id INTO payment_a;
  INSERT INTO inventory_balances(tenant_id,product_id,qty_on_hand)
    VALUES(a,p_a,1) ON CONFLICT (product_id) DO UPDATE SET qty_on_hand=EXCLUDED.qty_on_hand RETURNING product_id INTO balance_a;
  INSERT INTO inventory_movements(tenant_id,product_id,movement_type,delta,qty_before,qty_after)
    VALUES(a,p_a,'adjustment',1,0,1) RETURNING id INTO movement_a;
  INSERT INTO cash_movements(tenant_id,cash_session_id,movement_type,amount)
    VALUES(a,session_a,'supply',1) RETURNING id INTO cash_move_a;
  INSERT INTO cash_session_reconciliations(tenant_id,cash_session_id,method,expected_amount,declared_amount,difference_amount)
    VALUES(a,session_a,'cash',10,10,0);
  BEGIN
    UPDATE products SET category_id=cat_b WHERE id=p_a;
    RAISE EXCEPTION 'cross-tenant products.category_id accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core53_products_category_id_fk' THEN RAISE; END IF;
  END;
  BEGIN
    UPDATE inventory_balances SET tenant_id=b WHERE product_id=balance_a;
    RAISE EXCEPTION 'cross-tenant inventory_balances.tenant_id accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core53_inventory_balances_product_id_fk' THEN RAISE; END IF;
  END;
  BEGIN
    UPDATE inventory_movements SET product_id=p_b WHERE id=movement_a;
    RAISE EXCEPTION 'cross-tenant inventory_movements.product_id accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core53_inventory_movements_product_id_fk' THEN RAISE; END IF;
  END;
  BEGIN
    UPDATE cash_sessions SET cash_register_id=reg_b WHERE id=session_a;
    RAISE EXCEPTION 'cross-tenant cash_sessions.cash_register_id accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core53_cash_sessions_cash_register_id_fk' THEN RAISE; END IF;
  END;
  BEGIN
    UPDATE sales SET cash_session_id=session_b WHERE id=sale_a;
    RAISE EXCEPTION 'cross-tenant sales.cash_session_id accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core53_sales_cash_session_id_fk' THEN RAISE; END IF;
  END;
  BEGIN
    UPDATE sales SET customer_id=customer_b WHERE id=sale_a;
    RAISE EXCEPTION 'cross-tenant sales.customer_id accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core53_sales_customer_id_fk' THEN RAISE; END IF;
  END;
  BEGIN
    UPDATE sale_items SET sale_id=sale_b WHERE id=item_a;
    RAISE EXCEPTION 'cross-tenant sale_items.sale_id accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core53_sale_items_sale_id_fk' THEN RAISE; END IF;
  END;
  BEGIN
    UPDATE sale_items SET product_id=p_b WHERE id=item_a;
    RAISE EXCEPTION 'cross-tenant sale_items.product_id accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core53_sale_items_product_id_fk' THEN RAISE; END IF;
  END;
  BEGIN
    UPDATE payments SET sale_id=sale_b WHERE id=payment_a;
    RAISE EXCEPTION 'cross-tenant payments.sale_id accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core53_payments_sale_id_fk' THEN RAISE; END IF;
  END;
  BEGIN
    UPDATE cash_movements SET cash_session_id=session_b WHERE id=cash_move_a;
    RAISE EXCEPTION 'cross-tenant cash_movements.cash_session_id accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core53_cash_movements_cash_session_id_fk' THEN RAISE; END IF;
  END;
  BEGIN
    UPDATE cash_session_reconciliations SET cash_session_id=session_b WHERE cash_session_id=session_a;
    RAISE EXCEPTION 'cross-tenant cash_session_reconciliations.cash_session_id accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core53_cash_session_reconciliations_cash_session_id_fk' THEN RAISE; END IF;
  END;
  -- Existing nullable relationship semantics remain: only reference is nulled.
  DELETE FROM categories WHERE id=cat_a;
  IF NOT EXISTS (SELECT 1 FROM products WHERE id=p_a AND tenant_id=a AND category_id IS NULL) THEN
    RAISE EXCEPTION 'category deletion changed tenant or failed to clear category';
  END IF;
  DELETE FROM customers WHERE id=customer_a;
  IF NOT EXISTS (SELECT 1 FROM sales WHERE id=sale_a AND tenant_id=a AND customer_id IS NULL) THEN
    RAISE EXCEPTION 'customer deletion changed tenant or failed to clear customer';
  END IF;
END $test$;
ROLLBACK;
