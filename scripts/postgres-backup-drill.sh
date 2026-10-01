#!/usr/bin/env bash
set -euo pipefail

umask 077

fail() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

require_env() {
  local name="$1"
  [ -n "${!name:-}" ] || fail "required environment variable is missing: $name"
}

require_cmd pg_dump
require_cmd pg_restore
require_cmd psql
require_cmd sha256sum
require_cmd date
require_cmd mkdir

require_env SOURCE_DATABASE_URL
require_env RESTORE_DATABASE_URL

if [ "$SOURCE_DATABASE_URL" = "$RESTORE_DATABASE_URL" ]; then
  fail "SOURCE_DATABASE_URL and RESTORE_DATABASE_URL must be different"
fi

if [ "${ALLOW_RESTORE_RESET:-}" != "1" ]; then
  fail "set ALLOW_RESTORE_RESET=1 to acknowledge destructive reset of the restore database"
fi

source_db="$(psql "$SOURCE_DATABASE_URL" -v ON_ERROR_STOP=1 -Atc 'SELECT current_database();')"
restore_db="$(psql "$RESTORE_DATABASE_URL" -v ON_ERROR_STOP=1 -Atc 'SELECT current_database();')"

[ -n "$source_db" ] || fail "could not determine source database name"
[ -n "$restore_db" ] || fail "could not determine restore database name"

if [ "$source_db" = "$restore_db" ]; then
  fail "source and restore connections resolve to the same database: $source_db"
fi

case "$restore_db" in
  *restore*|*backup*|*drill*|*validation*|*test*) ;;
  *)
    fail "restore database '$restore_db' does not look disposable; use a name containing restore, backup, drill, validation, or test"
    ;;
esac

backup_dir="${BACKUP_DIR:-artifacts/backups}"
mkdir -p "$backup_dir"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
dump_file="${BACKUP_FILE:-$backup_dir/sistemaemgo-${timestamp}.dump}"
checksum_file="${dump_file}.sha256"
evidence_file="${dump_file}.evidence.txt"

source_version="$(psql "$SOURCE_DATABASE_URL" -v ON_ERROR_STOP=1 -At -F '|' -c 'SELECT version, dirty FROM schema_migrations LIMIT 1;')"
[ -n "$source_version" ] || fail "source schema_migrations is missing or empty"

source_dirty="${source_version#*|}"
source_schema_version="${source_version%%|*}"
if [ "$source_dirty" != "f" ]; then
  fail "source database migration state is dirty"
fi
if [ "$source_schema_version" -lt 25 ]; then
  fail "source schema version $source_schema_version is below the current pilot baseline (25)"
fi

printf 'Creating logical backup from %s...\n' "$source_db"
pg_dump \
  "$SOURCE_DATABASE_URL" \
  --format=custom \
  --compress=9 \
  --no-owner \
  --no-privileges \
  --file="$dump_file"

[ -s "$dump_file" ] || fail "pg_dump created an empty backup file"
(
  cd "$(dirname "$dump_file")"
  sha256sum "$(basename "$dump_file")" > "$(basename "$checksum_file")"
  sha256sum -c "$(basename "$checksum_file")"
)

printf 'Resetting disposable restore database %s...\n' "$restore_db"
psql "$RESTORE_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
SQL

printf 'Restoring backup into %s...\n' "$restore_db"
pg_restore \
  --exit-on-error \
  --no-owner \
  --no-privileges \
  --dbname="$RESTORE_DATABASE_URL" \
  "$dump_file"

restore_version="$(psql "$RESTORE_DATABASE_URL" -v ON_ERROR_STOP=1 -At -F '|' -c 'SELECT version, dirty FROM schema_migrations LIMIT 1;')"
restore_dirty="${restore_version#*|}"
restore_schema_version="${restore_version%%|*}"

if [ "$restore_dirty" != "f" ]; then
  fail "restored database migration state is dirty"
fi

if [ "$restore_schema_version" != "$source_schema_version" ]; then
  fail "schema version mismatch after restore: source=$source_schema_version restore=$restore_schema_version"
fi

critical_tables="$(psql "$RESTORE_DATABASE_URL" -v ON_ERROR_STOP=1 -Atc "
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
      'product_fiscal_profiles','sale_item_fiscal_snapshots','invoice_item_tax_calculations'
    );
")"

if [ "$critical_tables" -lt 39 ]; then
  fail "restored database is missing critical tables ($critical_tables/39 found)"
fi

source_counts="$(psql "$SOURCE_DATABASE_URL" -v ON_ERROR_STOP=1 -At -F '|' -c "
  SELECT
    (SELECT count(*) FROM companies),
    (SELECT count(*) FROM users),
    (SELECT count(*) FROM products),
    (SELECT count(*) FROM sales),
    (SELECT count(*) FROM audit_logs);
")"
restore_counts="$(psql "$RESTORE_DATABASE_URL" -v ON_ERROR_STOP=1 -At -F '|' -c "
  SELECT
    (SELECT count(*) FROM companies),
    (SELECT count(*) FROM users),
    (SELECT count(*) FROM products),
    (SELECT count(*) FROM sales),
    (SELECT count(*) FROM audit_logs);
")"

if [ "${STRICT_ROW_COUNTS:-0}" = "1" ] && [ "$source_counts" != "$restore_counts" ]; then
  fail "strict row-count validation failed: source=$source_counts restore=$restore_counts"
fi

cat > "$evidence_file" <<EOF_EVIDENCE
SistemaEmGo PostgreSQL backup/restore drill
UTC timestamp: $timestamp
Source database: $source_db
Restore database: $restore_db
Schema version: $restore_schema_version
Critical tables: $critical_tables/39
Source counts (companies|users|products|sales|audit_logs): $source_counts
Restore counts (companies|users|products|sales|audit_logs): $restore_counts
Strict row counts: ${STRICT_ROW_COUNTS:-0}
Backup file: $dump_file
Checksum file: $checksum_file
Result: PASS
EOF_EVIDENCE

printf '\nPASS: backup/restore drill completed successfully.\n'
printf 'Backup: %s\n' "$dump_file"
printf 'Checksum: %s\n' "$checksum_file"
printf 'Evidence: %s\n' "$evidence_file"
printf 'Restore database left intact for manual inspection: %s\n' "$restore_db"
