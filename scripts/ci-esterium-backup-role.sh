#!/usr/bin/env bash
# Only the disposable hosted integration job may create this fixed-name fixture.
set -euo pipefail
if [[ "${GITHUB_ACTIONS:-}" != true || "${RUNNER_ENVIRONMENT:-}" != github-hosted ]]; then
  echo "Refusing fixed-name backup-role fixture outside hosted CI" >&2
  exit 1
fi
db() { docker compose exec -T db psql -X -v ON_ERROR_STOP=1 -U sistemaemgo "$@"; }
stage=scripts/stage-esterium-postgres-backup-role.sql
expect_failure() {
  if "$@"; then
    echo "Expected refusal, got success" >&2
    exit 1
  fi
}
# Never replace a pre-existing database or identity.
db -d postgres -c "CREATE ROLE esterium_app NOLOGIN"
db -d postgres -c "CREATE DATABASE esterium OWNER esterium_app"
db -d esterium <<'SQL'
SET ROLE esterium_app;
CREATE TABLE public.schema_migrations (version bigint NOT NULL, dirty boolean NOT NULL);
INSERT INTO public.schema_migrations VALUES (54, false);
CREATE TABLE public.backup_fixture (id serial PRIMARY KEY, payload text NOT NULL);
INSERT INTO public.backup_fixture(payload) VALUES ('synthetic');
RESET ROLE;
SQL
expect_failure db -d postgres < "$stage"
db -d esterium -c 'GRANT CREATE ON SCHEMA public TO PUBLIC'
expect_failure db -d esterium < "$stage"
db -d esterium -c 'REVOKE CREATE ON SCHEMA public FROM PUBLIC'
[[ "$(db -d postgres -Atc "SELECT count(*) FROM pg_roles WHERE rolname='esterium_backup'")" == 0 ]]
db -d esterium -c 'ALTER TABLE public.backup_fixture ENABLE ROW LEVEL SECURITY'
expect_failure db -d esterium < "$stage"
db -d esterium -c 'ALTER TABLE public.backup_fixture DISABLE ROW LEVEL SECURITY'
db -d esterium < "$stage"
expect_failure db -d esterium < "$stage"
db -d esterium <<'SQL'
SET ROLE esterium_app;
CREATE TABLE public.future_fixture (id serial PRIMARY KEY, payload text);
INSERT INTO public.future_fixture(payload) VALUES ('future');
RESET ROLE;
DO $test$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_roles WHERE rolname='esterium_backup'
      AND NOT (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls OR rolinherit)
  ) THEN RAISE EXCEPTION 'Unsafe staged role'; END IF;
  IF NOT has_table_privilege('esterium_backup','public.future_fixture','SELECT')
     OR has_table_privilege('esterium_backup','public.future_fixture','INSERT,UPDATE,DELETE,TRUNCATE')
     OR NOT has_sequence_privilege('esterium_backup','public.future_fixture_id_seq','SELECT')
     OR has_sequence_privilege('esterium_backup','public.future_fixture_id_seq','USAGE,UPDATE')
  THEN RAISE EXCEPTION 'Unsafe default privileges'; END IF;
END $test$;
SET ROLE esterium_backup;
SELECT payload FROM public.backup_fixture;
SELECT payload FROM public.future_fixture;
RESET ROLE;
SQL
expect_failure db -d esterium -c "SET ROLE esterium_backup; INSERT INTO public.backup_fixture(payload) VALUES ('denied')"
expect_failure db -d esterium -c 'SET ROLE esterium_backup; CREATE TABLE public.denied(id int)'
# Exercise pg_dump permissions without enabling the staged role or using a password.
docker compose exec -T db pg_dump -U sistemaemgo --role=esterium_backup \
  --format=custom --no-owner --no-privileges esterium |
  docker compose exec -T db pg_restore --list | grep 'TABLE DATA public backup_fixture'
db -d postgres -c 'DROP DATABASE esterium'
db -d postgres -c 'DROP ROLE esterium_backup'
db -d postgres -c 'DROP ROLE esterium_app'
echo 'PASS: backup role staging, effective rights, default grants, rollback and archive coverage'
