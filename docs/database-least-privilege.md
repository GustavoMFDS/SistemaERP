# PostgreSQL least-privilege roles

Production-like deployments should not run the API with the same PostgreSQL role that owns the database or executes migrations.

Use separate credentials for separate responsibilities.

## Recommended roles

### Application role

Used by the SistemaEmGo API at runtime.

It should:

- log in;
- connect to the application database;
- use the application schema;
- SELECT/INSERT/UPDATE/DELETE only on the tables the application needs;
- use required sequences.

It should **not** have:

- SUPERUSER;
- CREATEDB;
- CREATEROLE;
- REPLICATION;
- BYPASSRLS;
- database-level CREATE;
- schema-level CREATE;
- ownership of the production database/schema.

The pilot preflight now checks these restrictions through `DATABASE_URL`.

### Migrator/owner role

Used only by controlled deployment/migration automation.

It may own the database/schema and apply migrations, but its credentials must not be injected into the long-running API container/process.

### Backup role

Used by backup automation.

Prefer a read-only login that can connect and read all application tables required by `pg_dump`. It must not be the API role and should not be a superuser unless a specific backup technology genuinely requires it.

## Example PostgreSQL setup

Adapt names and ownership to the target environment. Execute as an administrative/migrator account, not as the application runtime role.

```sql
CREATE ROLE sistemaemgo_app LOGIN;
CREATE ROLE sistemaemgo_backup LOGIN;

GRANT CONNECT ON DATABASE sistemaemgo TO sistemaemgo_app, sistemaemgo_backup;

\c sistemaemgo

GRANT USAGE ON SCHEMA public TO sistemaemgo_app, sistemaemgo_backup;

GRANT SELECT, INSERT, UPDATE, DELETE
ON ALL TABLES IN SCHEMA public
TO sistemaemgo_app;

GRANT USAGE, SELECT
ON ALL SEQUENCES IN SCHEMA public
TO sistemaemgo_app;

GRANT SELECT
ON ALL TABLES IN SCHEMA public
TO sistemaemgo_backup;
```

Default privileges must be configured by the schema owner/migrator so future migration-created tables remain usable:

```sql
ALTER DEFAULT PRIVILEGES FOR ROLE <schema_owner> IN SCHEMA public
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO sistemaemgo_app;

ALTER DEFAULT PRIVILEGES FOR ROLE <schema_owner> IN SCHEMA public
GRANT USAGE, SELECT ON SEQUENCES TO sistemaemgo_app;

ALTER DEFAULT PRIVILEGES FOR ROLE <schema_owner> IN SCHEMA public
GRANT SELECT ON TABLES TO sistemaemgo_backup;
```

Do not grant `CREATE` on the database or `public` schema to `sistemaemgo_app`.

## Credential handling

- keep application, migrator, and backup credentials separate;
- rotate them independently;
- inject them through the deployment secret store;
- never commit passwords or full production connection strings;
- keep migration credentials unavailable to the API runtime;
- keep backup credentials unavailable to the API runtime.

## Pilot verification

Run:

```bash
APP_ENV=staging \
API_BASE_URL=https://... \
DATABASE_URL='postgres://sistemaemgo_app:...@.../sistemaemgo?sslmode=verify-full' \
PILOT_TENANT_ID=... \
FISCAL_PROVIDER=disabled \
bash scripts/pilot-readiness.sh
```

The preflight fails if the runtime DB role has elevated cluster privileges or DDL capability on the target database/schema.

This verifies the application credential. Migration and backup credentials still need independent review in the target infrastructure.
