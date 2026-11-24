-- Dedicated CI database, never a store database. All fixture rows rolled back.
BEGIN;
DO $test$
DECLARE
  a uuid; b uuid; actor uuid; member uuid; cashier uuid; reg_a uuid; reg_b uuid;
  session_a uuid; session_b uuid; sale_a uuid; sale_b uuid; ledger uuid;
  actual text;
BEGIN
  IF current_database() <> 'sistemaemgo_core54_ci' THEN
    RAISE EXCEPTION 'requires dedicated core54 CI database';
  END IF;
  INSERT INTO companies(legal_name,trade_name,cnpj) VALUES ('CI Finance A','CI Finance A','00000000005411') RETURNING id INTO a;
  INSERT INTO companies(legal_name,trade_name,cnpj) VALUES ('CI Finance B','CI Finance B','00000000005412') RETURNING id INTO b;
  SELECT id INTO STRICT actor FROM users WHERE email='admin@sistema.local';
  SELECT id INTO STRICT cashier FROM roles WHERE name='cashier';
  INSERT INTO users(email,name,password_hash) VALUES ('core54-member@example.invalid','CI member','fixture-only') RETURNING id INTO member;
  INSERT INTO user_tenants(user_id,tenant_id) VALUES(actor,a),(actor,b),(member,a);
  INSERT INTO cash_registers(tenant_id,name) VALUES(a,'Finance A') RETURNING id INTO reg_a;
  INSERT INTO cash_registers(tenant_id,name) VALUES(b,'Finance B') RETURNING id INTO reg_b;
  INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,status)
    VALUES(a,reg_a,actor,'closed') RETURNING id INTO session_a;
  INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,status)
    VALUES(b,reg_b,actor,'closed') RETURNING id INTO session_b;
  INSERT INTO sales(tenant_id,cash_session_id,subtotal,total,created_by_user_id)
    VALUES(a,session_a,10,10,actor) RETURNING id INTO sale_a;
  INSERT INTO sales(tenant_id,cash_session_id,subtotal,total,created_by_user_id)
    VALUES(b,session_b,10,10,actor) RETURNING id INTO sale_b;
  INSERT INTO ledger_entries(tenant_id,entry_type,sale_id,cash_session_id,amount_net)
    VALUES(a,'sale',sale_a,session_a,10) RETURNING id INTO ledger;
  INSERT INTO idempotency_keys(tenant_id,operation,idem_key,request_hash,sale_id,total)
    VALUES(a,'core54.ci','valid','fixture',sale_a,10);
  INSERT INTO user_tenant_roles(user_id,tenant_id,role_id) VALUES(member,a,cashier);
  BEGIN
    UPDATE ledger_entries SET sale_id=sale_b WHERE id=ledger;
    RAISE EXCEPTION 'cross-tenant ledger sale accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core54_ledger_sale_fk' THEN RAISE; END IF;
  END;
  BEGIN
    UPDATE ledger_entries SET cash_session_id=session_b WHERE id=ledger;
    RAISE EXCEPTION 'cross-tenant ledger cash accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core54_ledger_cash_fk' THEN RAISE; END IF;
  END;
  BEGIN
    INSERT INTO idempotency_keys(tenant_id,operation,idem_key,request_hash,sale_id,total)
      VALUES(a,'core54.ci','invalid','fixture',sale_b,10);
    RAISE EXCEPTION 'cross-tenant idempotency accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core54_idempotency_sale_fk' THEN RAISE; END IF;
  END;
  BEGIN
    INSERT INTO user_tenant_roles(user_id,tenant_id,role_id) VALUES(member,b,cashier);
    RAISE EXCEPTION 'role without membership accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core54_roles_membership_fk' THEN RAISE; END IF;
  END;
  BEGIN
    DELETE FROM sales WHERE id=sale_a;
    RAISE EXCEPTION 'sale deletion with live idempotency record accepted';
  EXCEPTION WHEN foreign_key_violation THEN
    GET STACKED DIAGNOSTICS actual = CONSTRAINT_NAME;
    IF actual <> 'core54_idempotency_sale_fk' THEN RAISE; END IF;
  END;
  -- Suspend does not erase a grant; hard membership removal must not leave
  -- latent roles that can revive if the same user is linked later.
  UPDATE user_tenants SET active=false WHERE user_id=member AND tenant_id=a;
  IF NOT EXISTS(SELECT 1 FROM user_tenant_roles WHERE user_id=member AND tenant_id=a) THEN
    RAISE EXCEPTION 'suspension destroyed role history';
  END IF;
  DELETE FROM user_tenants WHERE user_id=member AND tenant_id=a;
  INSERT INTO user_tenants(user_id,tenant_id) VALUES(member,a);
  IF EXISTS(SELECT 1 FROM user_tenant_roles WHERE user_id=member AND tenant_id=a) THEN
    RAISE EXCEPTION 'removed role resurrected after relink';
  END IF;
  DELETE FROM idempotency_keys WHERE tenant_id=a AND idem_key='valid';
  DELETE FROM sales WHERE id=sale_a;
  DELETE FROM cash_sessions WHERE id=session_a;
  IF NOT EXISTS(SELECT 1 FROM ledger_entries WHERE id=ledger AND tenant_id=a
    AND sale_id IS NULL AND cash_session_id IS NULL AND amount_net=10) THEN
    RAISE EXCEPTION 'ledger history or tenant changed on unlink';
  END IF;
END $test$;
ROLLBACK;
