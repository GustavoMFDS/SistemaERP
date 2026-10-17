#!/usr/bin/env bash
set -euo pipefail

# CI-only: prove PostgreSQL backup recovers in an entirely independent
# server, not only a second database of the existing server.
# Only a container created by this run may be removed.
if [[ "$CI" != "true" || "$GITHUB_ACTIONS" != "true" ||
      "$RUNNER_ENVIRONMENT" != "github-hosted" ]]; then
  echo "Restricted to a disposable GitHub-hosted CI runner." >&2
  exit 2
fi

source_db=sistemaemgo
source_user=sistemaemgo
printf -v restore_container 'esterium_cross_restore_%s_%s' "$GITHUB_RUN_ID" "$GITHUB_RUN_ATTEMPT"
if docker container inspect "$restore_container" >/dev/null 2>&1; then
  echo "Refusing: an identically named container already exists." >&2
  exit 2
fi

source_psql() {
  docker compose exec -T db psql -U "$source_user" -d "$source_db" -v ON_ERROR_STOP=1 -At -F '|' -c "$1"
}

if [[ "$(source_psql 'SELECT current_database();')" != "$source_db" ]]; then
  echo "Refusing an unexpected source database." >&2
  exit 2
fi
migration_query='SELECT version, dirty FROM schema_migrations LIMIT 1;'
source_version="$(source_psql "$migration_query")"
if [[ ! "$source_version" =~ ^[0-9]+\|f$ ]]; then
  echo "Missing or dirty source migration; refusing the restore test." >&2
  exit 1
fi
count_query="SELECT
  (SELECT count(*) FROM companies),
  (SELECT count(*) FROM users),
  (SELECT count(*) FROM products),
  (SELECT count(*) FROM sales),
  (SELECT count(*) FROM audit_logs),
  (SELECT count(*) FROM fiscal_document_sequences),
  (SELECT count(*) FROM sale_fiscal_intents);"
source_counts="$(source_psql "$count_query")"

# Canonical order-independent hashes detect altered values even if all
# table row counts happen to match. Only synthetic CI data is inspected.
content_query="SELECT
  (SELECT md5(COALESCE(string_agg(md5(row_to_json(t)::text), ',' ORDER BY md5(row_to_json(t)::text)), '')) FROM companies t),
  (SELECT md5(COALESCE(string_agg(md5(row_to_json(t)::text), ',' ORDER BY md5(row_to_json(t)::text)), '')) FROM users t),
  (SELECT md5(COALESCE(string_agg(md5(row_to_json(t)::text), ',' ORDER BY md5(row_to_json(t)::text)), '')) FROM products t),
  (SELECT md5(COALESCE(string_agg(md5(row_to_json(t)::text), ',' ORDER BY md5(row_to_json(t)::text)), '')) FROM sales t),
  (SELECT md5(COALESCE(string_agg(md5(row_to_json(t)::text), ',' ORDER BY md5(row_to_json(t)::text)), '')) FROM audit_logs t),
  (SELECT md5(COALESCE(string_agg(md5(row_to_json(t)::text), ',' ORDER BY md5(row_to_json(t)::text)), '')) FROM fiscal_document_sequences t),
  (SELECT md5(COALESCE(string_agg(md5(row_to_json(t)::text), ',' ORDER BY md5(row_to_json(t)::text)), '')) FROM sale_fiscal_intents t);"
source_fingerprint="$(source_psql "$content_query")"

archive="$(mktemp /tmp/esterium-cross-instance.XXXXXXXX.dump)"
checksum="$archive.sha256"
created=false
cleanup() {
  rm -f -- "$archive" "$checksum"
  if [[ "$created" == true ]]; then
    docker rm -f "$restore_container" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

docker compose exec -T db pg_dump -U "$source_user" -d "$source_db" --format=custom --compress=6 --no-owner --no-privileges > "$archive"
test -s "$archive" || { echo "Backup archive is empty." >&2; exit 1; }
sha256sum "$archive" > "$checksum"
sha256sum --check --status "$checksum"
docker compose exec -T db pg_restore --list < "$archive" >/dev/null

# Second instance shares no network, storage volume or process with source.
# Initdb uses a temporary UNIX-socket-only server. TCP readiness below
# waits for the final post-entrypoint server, avoiding shutdown races.
docker run -d --name "$restore_container" --network none --shm-size=128m -e POSTGRES_DB="$source_db" -e POSTGRES_USER="$source_user" -e POSTGRES_PASSWORD='ci-disposable-second-instance-only' postgres:16 >/dev/null
created=true
ready=false
for _ in $(seq 1 40); do
  if docker exec "$restore_container" pg_isready -h 127.0.0.1 -U "$source_user" -d "$source_db" >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 1
done
if [[ "$ready" != true ]]; then
  echo "Second isolated PostgreSQL instance did not become ready." >&2
  exit 1
fi

docker exec -i "$restore_container" pg_restore -U "$source_user" --exit-on-error --no-owner --no-privileges --dbname="$source_db" < "$archive"
restored_psql() {
  docker exec "$restore_container" psql -U "$source_user" -d "$source_db" -v ON_ERROR_STOP=1 -At -F '|' -c "$1"
}
if [[ "$(restored_psql "$migration_query")" != "$source_version" ||
      "$(restored_psql "$count_query")" != "$source_counts" ]]; then
  echo "Migration state or critical row counts differ after restore." >&2
  exit 1
fi
if [[ "$(restored_psql "$content_query")" != "$source_fingerprint" ]]; then
  echo "Critical business or fiscal content fingerprints differ after restore." >&2
  exit 1
fi
table_query="SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN (
'companies','users','products','sales','sale_items','payments','audit_logs','nfce_configs',
'fiscal_document_sequences','sale_fiscal_intents','schema_migrations');"
if [[ "$(restored_psql "$table_query")" != "11" ]]; then
  echo "One or more critical business/fiscal schema tables are missing." >&2
  exit 1
fi
echo "PASS: PostgreSQL backup restored on independent, network-isolated CI server; migrations, critical counts and content fingerprints match."
