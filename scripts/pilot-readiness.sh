#!/usr/bin/env bash
set -euo pipefail

failures=0
warnings=0

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  failures=$((failures + 1))
}

warn() {
  printf 'WARN: %s\n' "$1" >&2
  warnings=$((warnings + 1))
}

pass() {
  printf 'PASS: %s\n' "$1"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "required command not found: $1"
  fi
}

require_env() {
  local name="$1"
  if [ -z "${!name:-}" ]; then
    fail "required environment variable is missing: $name"
  fi
}

require_cmd curl
require_cmd psql
require_env API_BASE_URL
require_env DATABASE_URL
require_env PILOT_TENANT_ID

if [ "$failures" -gt 0 ]; then
  exit 1
fi

base="${API_BASE_URL%/}"

case "$base" in
  https://*)
    pass "API_BASE_URL uses HTTPS"
    ;;
  *)
    fail "API_BASE_URL must use HTTPS for a production-like pilot"
    ;;
esac

case "$DATABASE_URL" in
  *"sslmode=verify-full"*)
    pass "DATABASE_URL requires PostgreSQL certificate and hostname verification"
    ;;
  *)
    fail "DATABASE_URL must use sslmode=verify-full for a production-like pilot"
    ;;
esac

read -r role_super role_createdb role_createrole role_replication role_bypassrls < <(
  psql "$DATABASE_URL" -At -F ' ' -c "
    SELECT rolsuper, rolcreatedb, rolcreaterole, rolreplication, rolbypassrls
    FROM pg_roles
    WHERE rolname=current_user;
  "
)
if [ "$role_super" = "t" ] || [ "$role_createdb" = "t" ] || [ "$role_createrole" = "t" ] || [ "$role_replication" = "t" ] || [ "$role_bypassrls" = "t" ]; then
  fail "application database role has elevated cluster privileges (superuser/createdb/createrole/replication/bypassrls)"
else
  pass "application database role has no elevated cluster privileges"
fi

database_create="$(psql "$DATABASE_URL" -At -c "SELECT has_database_privilege(current_user, current_database(), 'CREATE');")"
if [ "$database_create" = "t" ]; then
  fail "application database role can CREATE objects at database level; use a separate migrator/owner role"
else
  pass "application database role cannot CREATE at database level"
fi

schema_create="$(psql "$DATABASE_URL" -At -c "SELECT has_schema_privilege(current_user, 'public', 'CREATE');")"
if [ "$schema_create" = "t" ]; then
  fail "application database role can CREATE objects in public schema; use a separate migrator/owner role"
else
  pass "application database role cannot CREATE in public schema"
fi

case "${APP_ENV:-}" in
  staging|prod|production)
    pass "APP_ENV is production-like (${APP_ENV})"
    ;;
  *)
    fail "APP_ENV must be staging, prod, or production for pilot readiness"
    ;;
esac

if [ "${FISCAL_PROVIDER:-disabled}" != "disabled" ]; then
  fail "FISCAL_PROVIDER must remain disabled until a SEFAZ-ready provider is approved"
else
  pass "fiscal provider is disabled"
fi

if [ "${ALLOW_DEMO_SEED:-false}" = "true" ] || [ "${ALLOW_DEMO_SEED:-0}" = "1" ]; then
  fail "ALLOW_DEMO_SEED must be disabled"
else
  pass "demo seed flag is disabled"
fi

if curl --fail --silent --show-error "${base}/health/live" >/dev/null; then
  pass "liveness endpoint"
else
  fail "liveness endpoint failed"
fi

if curl --fail --silent --show-error "${base}/health/ready" >/dev/null; then
  pass "readiness endpoint (PostgreSQL + Redis)"
else
  fail "readiness endpoint failed"
fi

read -r version dirty < <(
  psql "$DATABASE_URL" -At -F ' ' -c "SELECT version, dirty FROM schema_migrations LIMIT 1;"
)
if [ "${dirty:-}" != "f" ]; then
  fail "database migration state is dirty"
elif [ "${version:-0}" -lt 30 ]; then
  fail "database migration version ${version:-unknown} is below required pilot version 30"
else
  pass "database schema version ${version} is clean"
fi

critical_tables="$(
  psql "$DATABASE_URL" -At -c "
    SELECT count(*)
    FROM information_schema.tables
    WHERE table_schema='public'
      AND table_name IN (
        'companies','users','roles','user_tenants','user_tenant_roles','permissions','role_permissions',
        'products','inventory_balances','inventory_movements',
        'cash_registers','cash_sessions','cash_movements','cash_session_reconciliations',
        'sales','sale_items','payments','idempotency_keys',
        'suppliers','purchases','purchase_items','purchase_receipts','purchase_receipt_items',
        'accounts_payable','procurement_idempotency_keys',
        'sale_returns','sale_return_items','return_idempotency_keys',
        'return_refunds','payment_reconciliations','payment_reconciliation_adjustments','finance_idempotency_keys',
        'ledger_entries','audit_logs','nfce_configs','fiscal_document_sequences',
        'product_fiscal_profiles','sale_item_fiscal_snapshots','invoice_item_tax_calculations','invoice_fiscal_events',
        'nfce_number_inutilizations','opening_stock_batches'
      );
  "
)"
if [ "$critical_tables" -lt 42 ]; then
  fail "one or more critical pilot tables are missing ($critical_tables/42 found)"
else
  pass "critical pilot tables are present"
fi

immutable_triggers="$(
  psql "$DATABASE_URL" -At -c "
    SELECT count(*)
    FROM pg_trigger
    WHERE NOT tgisinternal
      AND tgname IN (
        'idempotency_keys_immutable',
        'procurement_idempotency_keys_immutable',
        'return_idempotency_keys_immutable',
        'finance_idempotency_keys_immutable'
      );
  "
)"
if [ "$immutable_triggers" -lt 4 ]; then
  fail "one or more idempotency immutability triggers are missing ($immutable_triggers/4 found)"
else
  pass "idempotency immutability triggers are present"
fi

reconciliation_history_triggers="$(
  psql "$DATABASE_URL" -At -c "
    SELECT count(*)
    FROM pg_trigger
    WHERE NOT tgisinternal
      AND tgname IN (
        'payment_reconciliations_immutable',
        'payment_reconciliation_adjustments_immutable'
      );
  "
)"
if [ "$reconciliation_history_triggers" -lt 2 ]; then
  fail "payment reconciliation history immutability is incomplete ($reconciliation_history_triggers/2 triggers found)"
else
  pass "payment reconciliation history is protected against update/delete"
fi

retention_indexes="$(
  psql "$DATABASE_URL" -At -c "
    SELECT count(*)
    FROM pg_indexes
    WHERE schemaname='public'
      AND indexname IN (
        'idempotency_keys_created_at_idx',
        'procurement_idempotency_created_at_idx',
        'return_idempotency_created_at_idx',
        'finance_idempotency_created_at_idx'
      );
  "
)"
if [ "$retention_indexes" -lt 4 ]; then
  fail "one or more idempotency retention indexes are missing ($retention_indexes/4 found)"
else
  pass "idempotency retention indexes are present"
fi

duplicate_open="$(
  psql "$DATABASE_URL" -At -c "
    SELECT count(*)
    FROM (
      SELECT tenant_id, cash_register_id
      FROM cash_sessions
      WHERE status='open'
      GROUP BY tenant_id, cash_register_id
      HAVING count(*) > 1
    ) q;
  "
)"
if [ "$duplicate_open" != "0" ]; then
  fail "duplicate open cash sessions detected ($duplicate_open register(s))"
else
  pass "no duplicate open cash sessions"
fi

demo_users="$(
  psql "$DATABASE_URL" -At -c "
    SELECT count(*)
    FROM users
    WHERE lower(email) IN ('admin@sistema.local','gerente@sistema.local','caixa@sistema.local');
  "
)"
if [ "$demo_users" != "0" ]; then
  fail "development demo users are present ($demo_users); replace them before a real-store pilot"
else
  pass "development demo users are absent"
fi

tenant_count="$(psql "$DATABASE_URL" -At -c "SELECT count(*) FROM companies;")"
if [ "$tenant_count" -lt 1 ]; then
  fail "no tenant/company configured"
else
  pass "at least one tenant/company is configured"
fi

pilot_tenant_exists="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(*)
    FROM companies
    WHERE id = :'tenant_id'::uuid;
  "
)"
if [ "$pilot_tenant_exists" != "1" ]; then
  fail "PILOT_TENANT_ID does not identify exactly one configured tenant"
else
  pass "pilot tenant exists"
fi

pilot_identity_ok="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    WITH normalized AS (
      SELECT
        id,
        legal_name,
        upper(regexp_replace(cnpj, '[^A-Za-z0-9]', '', 'g')) AS normalized_cnpj
      FROM companies
      WHERE id=:'tenant_id'::uuid
    ),
    candidate AS (
      SELECT *
      FROM normalized
      WHERE NULLIF(btrim(legal_name), '') IS NOT NULL
        AND normalized_cnpj ~ '^[A-Z0-9]{12}[0-9]{2}$'
        AND normalized_cnpj NOT IN ('00000000000000','11111111111111')
    ),
    first_digit AS (
      SELECT
        c.*,
        CASE
          WHEN calc.remainder IN (0,1) THEN 0
          ELSE 11 - calc.remainder
        END AS dv1
      FROM candidate c
      CROSS JOIN LATERAL (
        SELECT mod(
          sum(
            (ascii(substr(c.normalized_cnpj, i, 1)) - 48)
            * (ARRAY[5,4,3,2,9,8,7,6,5,4,3,2])[i]
          ),
          11
        )::int AS remainder
        FROM generate_series(1, 12) AS i
      ) calc
    ),
    second_digit AS (
      SELECT
        f.*,
        CASE
          WHEN calc.remainder IN (0,1) THEN 0
          ELSE 11 - calc.remainder
        END AS dv2
      FROM first_digit f
      CROSS JOIN LATERAL (
        SELECT mod(
          sum(
            (ascii(substr(f.normalized_cnpj || f.dv1::text, i, 1)) - 48)
            * (ARRAY[6,5,4,3,2,9,8,7,6,5,4,3,2])[i]
          ),
          11
        )::int AS remainder
        FROM generate_series(1, 13) AS i
      ) calc
    )
    SELECT count(*)
    FROM second_digit
    WHERE substr(normalized_cnpj, 13, 1)::int = dv1
      AND substr(normalized_cnpj, 14, 1)::int = dv2;
  "
)"
if [ "$pilot_identity_ok" != "1" ]; then
  fail "pilot tenant uses incomplete/demo identity or an invalid CNPJ check digit"
else
  pass "pilot tenant company identity is configured"
fi

active_products="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(*)
    FROM products
    WHERE tenant_id=:'tenant_id'::uuid
      AND active=true;
  "
)"
if [ "$active_products" -lt 1 ]; then
  fail "pilot tenant has no active products"
else
  pass "pilot tenant has active products"
fi

barcoded_products="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(*)
    FROM products
    WHERE tenant_id=:'tenant_id'::uuid
      AND active=true
      AND barcode IS NOT NULL
      AND length(trim(barcode)) > 0;
  "
)"
if [ "$barcoded_products" -lt 1 ]; then
  fail "pilot tenant has no active barcoded product for scanner validation"
else
  pass "pilot tenant has active barcoded products"
fi

sellable_stock="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(*)
    FROM inventory_balances b
    JOIN products p
      ON p.id=b.product_id
     AND p.tenant_id=b.tenant_id
    WHERE b.tenant_id=:'tenant_id'::uuid
      AND p.active=true
      AND b.qty_on_hand > 0;
  "
)"
if [ "$sellable_stock" -lt 1 ]; then
  fail "pilot tenant has no active product with positive opening stock"
else
  pass "pilot tenant has sellable opening stock"
fi

invalid_pricing="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(*)
    FROM products
    WHERE tenant_id=:'tenant_id'::uuid
      AND active=true
      AND promo_price IS NOT NULL
      AND promo_price > price_cash;
  "
)"
if [ "$invalid_pricing" != "0" ]; then
  fail "pilot tenant has active products with promo_price above price_cash ($invalid_pricing)"
else
  pass "pilot tenant promotional pricing is consistent"
fi

active_registers="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(*)
    FROM cash_registers
    WHERE tenant_id=:'tenant_id'::uuid
      AND active=true;
  "
)"
if [ "$active_registers" -lt 1 ]; then
  fail "pilot tenant has no active cash register"
else
  pass "pilot tenant has an active cash register"
fi

pilot_open_sessions="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(*)
    FROM cash_sessions
    WHERE tenant_id=:'tenant_id'::uuid
      AND status='open';
  "
)"
if [ "$pilot_open_sessions" != "0" ]; then
  fail "pilot tenant must start with no open cash session ($pilot_open_sessions found)"
else
  pass "pilot tenant starts with a clean cash-session baseline"
fi

active_suppliers="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(*)
    FROM suppliers
    WHERE tenant_id=:'tenant_id'::uuid
      AND active=true;
  "
)"
if [ "$active_suppliers" -lt 1 ]; then
  fail "pilot tenant has no active supplier for procurement validation"
else
  pass "pilot tenant has active suppliers"
fi

active_user_count="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(DISTINCT u.id)
    FROM users u
    JOIN user_tenants ut ON ut.user_id=u.id
    WHERE ut.tenant_id=:'tenant_id'::uuid
      AND u.active=true;
  "
)"
if [ "$active_user_count" -lt 2 ]; then
  fail "pilot tenant has fewer than two active users; provision an operator plus an admin/manager"
else
  pass "pilot tenant has at least two active users"
fi

orphan_roles="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(*)
    FROM users u
    JOIN user_tenants ut ON ut.user_id=u.id
    LEFT JOIN user_tenant_roles utr
      ON utr.user_id=u.id
     AND utr.tenant_id=ut.tenant_id
    WHERE ut.tenant_id=:'tenant_id'::uuid
      AND u.active=true
      AND utr.user_id IS NULL;
  "
)"
if [ "$orphan_roles" != "0" ]; then
  fail "pilot tenant has active memberships without tenant-scoped roles ($orphan_roles)"
else
  pass "pilot tenant memberships have tenant-scoped roles"
fi

operator_count="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(DISTINCT u.id)
    FROM users u
    JOIN user_tenants ut
      ON ut.user_id=u.id
     AND ut.tenant_id=:'tenant_id'::uuid
    JOIN user_tenant_roles utr
      ON utr.user_id=u.id
     AND utr.tenant_id=ut.tenant_id
    JOIN role_permissions rp ON rp.role_id=utr.role_id
    JOIN permissions p ON p.id=rp.permission_id
    WHERE u.active=true
      AND p.code='sale:write';
  "
)"
if [ "$operator_count" -lt 1 ]; then
  fail "pilot tenant has no active operator with sale:write"
else
  pass "pilot tenant has an active sales operator"
fi

manager_count="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    SELECT count(*)
    FROM (
      SELECT u.id
      FROM users u
      JOIN user_tenants ut
        ON ut.user_id=u.id
       AND ut.tenant_id=:'tenant_id'::uuid
      JOIN user_tenant_roles utr
        ON utr.user_id=u.id
       AND utr.tenant_id=ut.tenant_id
      JOIN role_permissions rp ON rp.role_id=utr.role_id
      JOIN permissions p ON p.id=rp.permission_id
      WHERE u.active=true
        AND p.code IN ('finance:read','audit:read')
      GROUP BY u.id
      HAVING count(DISTINCT p.code)=2
    ) q;
  "
)"
if [ "$manager_count" -lt 1 ]; then
  fail "pilot tenant has no active responsible user with finance:read and audit:read"
else
  pass "pilot tenant has an active responsible user for finance and audit"
fi

separated_duties_count="$(
  psql "$DATABASE_URL" -At -v tenant_id="$PILOT_TENANT_ID" -c "
    WITH operator_users AS (
      SELECT DISTINCT u.id
      FROM users u
      JOIN user_tenants ut
        ON ut.user_id=u.id
       AND ut.tenant_id=:'tenant_id'::uuid
      JOIN user_tenant_roles utr
        ON utr.user_id=u.id
       AND utr.tenant_id=ut.tenant_id
      JOIN role_permissions rp ON rp.role_id=utr.role_id
      JOIN permissions p ON p.id=rp.permission_id
      WHERE u.active=true
        AND p.code='sale:write'
    ),
    responsible_users AS (
      SELECT u.id
      FROM users u
      JOIN user_tenants ut
        ON ut.user_id=u.id
       AND ut.tenant_id=:'tenant_id'::uuid
      JOIN user_tenant_roles utr
        ON utr.user_id=u.id
       AND utr.tenant_id=ut.tenant_id
      JOIN role_permissions rp ON rp.role_id=utr.role_id
      JOIN permissions p ON p.id=rp.permission_id
      WHERE u.active=true
        AND p.code IN ('finance:read','audit:read')
      GROUP BY u.id
      HAVING count(DISTINCT p.code)=2
    )
    SELECT count(*)
    FROM operator_users o
    CROSS JOIN responsible_users r
    WHERE o.id <> r.id;
  "
)"
if [ "$separated_duties_count" -lt 1 ]; then
  fail "pilot tenant needs distinct active operator and responsible-user accounts"
else
  pass "pilot tenant has separate operator and responsible-user accounts"
fi

printf '\nPilot readiness summary: %d failure(s), %d warning(s).\n' "$failures" "$warnings"
if [ "$failures" -gt 0 ]; then
  exit 1
fi
