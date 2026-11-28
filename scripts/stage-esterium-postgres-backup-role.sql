-- Esterium Windows maintenance: stage a dedicated PostgreSQL backup role.
-- Run locally as the authorized PostgreSQL administrator, against esterium,
-- with psql -X -v ON_ERROR_STOP=1 -f <approved-file>.
-- No password, login activation, service change, or backup is performed here.
\set ON_ERROR_STOP on
BEGIN;
SELECT pg_advisory_xact_lock(239, 1);

DO $preflight$
BEGIN
    IF current_database() <> 'esterium' THEN
        RAISE EXCEPTION 'Wrong database: expected esterium';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = current_user AND rolsuper) THEN
        RAISE EXCEPTION 'Authorized PostgreSQL administrator required';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_database d JOIN pg_roles r ON r.oid = d.datdba
        WHERE d.datname = current_database() AND r.rolname = 'esterium_app'
    ) THEN
        RAISE EXCEPTION 'Unexpected database owner';
    END IF;
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'esterium_backup') THEN
        RAISE EXCEPTION 'Backup role already exists; review manually without changing it';
    END IF;
    IF to_regclass('public.schema_migrations') IS NULL THEN
        RAISE EXCEPTION 'Missing migration state';
    END IF;
    IF (SELECT count(*) FROM public.schema_migrations) <> 1 OR
       NOT EXISTS (SELECT 1 FROM public.schema_migrations WHERE version > 0 AND NOT dirty) THEN
        RAISE EXCEPTION 'Migration state is not clean';
    END IF;
    IF EXISTS (
        SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
        JOIN pg_roles r ON r.oid = c.relowner
        WHERE n.nspname = 'public' AND c.relkind IN ('r','p','v','m','f','S')
          AND (r.rolname NOT IN ('esterium_app', current_user) OR c.relrowsecurity)
    ) THEN
        RAISE EXCEPTION 'Unexpected owner or row-security policy; review backup coverage';
    END IF;
END
$preflight$;

CREATE ROLE esterium_backup NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE
    NOREPLICATION NOBYPASSRLS NOINHERIT;
ALTER ROLE esterium_backup IN DATABASE esterium SET default_transaction_read_only = on;
GRANT CONNECT ON DATABASE esterium TO esterium_backup;
GRANT USAGE ON SCHEMA public TO esterium_backup;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO esterium_backup;
GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO esterium_backup;
ALTER DEFAULT PRIVILEGES FOR ROLE esterium_app IN SCHEMA public
    GRANT SELECT ON TABLES TO esterium_backup;
ALTER DEFAULT PRIVILEGES FOR ROLE esterium_app IN SCHEMA public
    GRANT SELECT ON SEQUENCES TO esterium_backup;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO esterium_backup;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON SEQUENCES TO esterium_backup;

DO $verify$
BEGIN
    IF has_database_privilege('esterium_backup', current_database(), 'CREATE') OR
       has_schema_privilege('esterium_backup', 'public', 'CREATE') OR
       EXISTS (
           SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
           WHERE n.nspname = 'public' AND c.relkind IN ('r','p','v','m','f')
             AND (NOT has_table_privilege('esterium_backup', c.oid, 'SELECT') OR
                  has_table_privilege('esterium_backup', c.oid, 'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER') OR
                  has_any_column_privilege('esterium_backup', c.oid, 'INSERT,UPDATE,REFERENCES'))
       ) OR EXISTS (
           SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
           WHERE n.nspname = 'public' AND c.relkind = 'S'
             AND (NOT has_sequence_privilege('esterium_backup', c.oid, 'SELECT') OR
                  has_sequence_privilege('esterium_backup', c.oid, 'USAGE,UPDATE'))
       ) OR EXISTS (
           SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
           WHERE n.nspname = 'public' AND p.prosecdef
             AND has_function_privilege('esterium_backup', p.oid, 'EXECUTE')
       ) THEN
        RAISE EXCEPTION 'Unexpected effective privilege or missing read access; rolling back';
    END IF;
END
$verify$;
COMMIT;
\echo 'PASS: staged NOLOGIN backup role; no credential or login activation performed.'
