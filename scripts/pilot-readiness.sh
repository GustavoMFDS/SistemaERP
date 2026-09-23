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

if [ "$failures" -gt 0 ]; then
  exit 1
fi

base="${API_BASE_URL%/}"

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
elif [ "${version:-0}" -lt 21 ]; then
  fail "database migration version ${version:-unknown} is below required pilot version 21"
else
  pass "database schema version ${version} is clean"
fi

critical_tables="$(
  psql "$DATABASE_URL" -At -c "
    SELECT count(*)
    FROM information_schema.tables
    WHERE table_schema='public'
      AND table_name IN (
        'companies','users','user_tenants','user_tenant_roles','products',
        'inventory_balances','inventory_movements','cash_sessions','cash_movements',
        'sales','sale_items','payments','suppliers','purchases','purchase_receipts',
        'sale_returns','sale_return_items','return_refunds','payment_reconciliations',
        'finance_ledger','audit_logs','idempotency_keys'
      );
  "
)"
if [ "$critical_tables" -lt 22 ]; then
  fail "one or more critical pilot tables are missing ($critical_tables/22 found)"
else
  pass "critical pilot tables are present"
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

active_user_count="$(psql "$DATABASE_URL" -At -c "SELECT count(*) FROM users WHERE active=true;")"
if [ "$active_user_count" -lt 2 ]; then
  warn "fewer than two active users; pilot should have at least an operator plus an admin/manager"
else
  pass "active users are provisioned"
fi

orphan_roles="$(
  psql "$DATABASE_URL" -At -c "
    SELECT count(*)
    FROM users u
    JOIN user_tenants ut ON ut.user_id=u.id
    LEFT JOIN user_tenant_roles utr ON utr.user_id=u.id AND utr.tenant_id=ut.tenant_id
    WHERE u.active=true AND utr.user_id IS NULL;
  "
)"
if [ "$orphan_roles" != "0" ]; then
  fail "active tenant memberships without tenant-scoped roles detected ($orphan_roles)"
else
  pass "active tenant memberships have tenant-scoped roles"
fi

printf '\nPilot readiness summary: %d failure(s), %d warning(s).\n' "$failures" "$warnings"
if [ "$failures" -gt 0 ]; then
  exit 1
fi
