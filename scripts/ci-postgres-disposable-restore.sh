#!/usr/bin/env bash
set -euo pipefail

# CI-only restoration evidence. Never DROP any database, schema or tenant.
# Both names and the compose service are fixed, not caller-controlled.
if [[ "$CI" != "true" || "$GITHUB_ACTIONS" != "true" ]]; then
  echo "Refusing: this disposable restore test is only allowed in GitHub CI." >&2
  exit 2
fi
for cmd in docker mktemp sha256sum grep; do
  command -v "$cmd" >/dev/null || { echo "Missing command: $cmd" >&2; exit 2; }
done

source_database=sistemaemgo
restore_database=sistemaemgo_restore_ci
[[ "$source_database" != "$restore_database" ]] || exit 2
pg() { docker compose exec -T db psql -U sistemaemgo -v ON_ERROR_STOP=1 "$@"; }

source_name="$(pg -d "$source_database" -At -c 'SELECT current_database();')"
if [[ "$source_name" != "$source_database" ]]; then
  echo "Refusing to dump unexpected database." >&2
  exit 2
fi
already_exists="$(pg -d postgres -At -c "SELECT count(*) FROM pg_database WHERE datname='$restore_database';")"
if [[ "$already_exists" != "0" ]]; then
  echo "Refusing to overwrite existing restore database." >&2
  exit 2
fi

archive="$(mktemp /tmp/esterium-restore-ci.XXXXXXXX.dump)"
hashfile="$archive.sha256"
trap 'rm -f -- "$archive" "$hashfile"' EXIT

docker compose exec -T db pg_dump -U sistemaemgo -d "$source_database" \
  --format=custom --compress=6 --no-owner --no-privileges > "$archive"
test -s "$archive" || { echo "pg_dump created an empty archive." >&2; exit 1; }
sha256sum "$archive" > "$hashfile"
sha256sum -c "$hashfile"
docker compose exec -T db pg_restore --list < "$archive" >/dev/null

source_version="$(pg -d "$source_database" -At -F '|' -c 'SELECT version, dirty FROM schema_migrations LIMIT 1;')"
if [[ ! "$source_version" =~ ^[0-9]+\|f$ ]]; then
  echo "Source has an invalid/dirty migration state." >&2
  exit 1
fi
snapshot_query="SELECT
  (SELECT count(*) FROM companies),
  (SELECT count(*) FROM users),
  (SELECT count(*) FROM products),
  (SELECT count(*) FROM sales),
  (SELECT count(*) FROM audit_logs),
  (SELECT count(*) FROM fiscal_document_sequences),
  (SELECT count(*) FROM sale_fiscal_intents);"
source_counts="$(pg -d "$source_database" -At -F '|' -c "$snapshot_query")"
# This source is the disposable CI database, not a shop's production database.
pg -d postgres -c "CREATE DATABASE $restore_database OWNER sistemaemgo;"
docker compose exec -T db pg_restore -U sistemaemgo --exit-on-error \
  --no-owner --no-privileges --dbname="$restore_database" < "$archive"

restore_version="$(pg -d "$restore_database" -At -F '|' -c 'SELECT version, dirty FROM schema_migrations LIMIT 1;')"
restore_counts="$(pg -d "$restore_database" -At -F '|' -c "$snapshot_query")"
if [[ "$source_version" != "$restore_version" ]]; then
  echo "Schema migration version or dirty flag differs after restore." >&2
  exit 1
fi
if [[ "$source_counts" != "$restore_counts" ]]; then
  echo "Critical company/user/product/sale/audit/fiscal counts differ after restore." >&2
  exit 1
fi
critical_tables="$(pg -d "$restore_database" -At -c "
SELECT count(*) FROM information_schema.tables WHERE table_schema='public'
  AND table_name IN (
    'companies','users','products','sales','sale_items','payments',
    'audit_logs','nfce_configs','fiscal_document_sequences',
    'sale_fiscal_intents','schema_migrations'
  );")"
if [[ "$critical_tables" != "11" ]]; then
  echo "The restored DB is missing critical tables ($critical_tables/11)." >&2
  exit 1
fi
echo "PASS: PostgreSQL archive restored to separate disposable CI database; schema and critical counts match."
# No DROP; the entire runner-owned Compose environment is discarded after CI.
