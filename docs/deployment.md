# Deployment and operations

This document describes technical deployment controls. It does not replace legal, accounting, DPO, or infrastructure approval.

## Validation commands

Frontend clean validation:

```bash
cd web
npm ci
npm run lint
npm run build
```

For the current hardening pass, `cd web && npm run build` was validated separately before backend/operations work. Re-run the frontend validation commands after frontend source or dependency changes.

Backend validation:

```bash
cd backend
gofmt -l .
go test ./...
go vet ./...
```

Migration validation on a clean database:

```bash
docker compose down -v
docker compose up -d db redis migrate
```

This is destructive for local Docker volumes. Use only in disposable local/staging validation environments, never against production volumes. Validate the schema by checking that all migration rows are present and spot-checking critical objects:

```bash
docker compose exec db psql -U sistemaemgo -d sistemaemgo -c "SELECT version, dirty FROM schema_migrations;"
docker compose exec db psql -U sistemaemgo -d sistemaemgo -c "\d idempotency_keys"
docker compose exec db psql -U sistemaemgo -d sistemaemgo -c "\d audit_logs"
docker compose exec db psql -U sistemaemgo -d sistemaemgo -c "\d data_subject_requests"
docker compose exec db psql -U sistemaemgo -d sistemaemgo -c "\d consent_records"
docker compose exec db psql -U sistemaemgo -d sistemaemgo -c "\d user_tenant_roles"
```

To verify app startup against the migrated database, inject production-like non-secret local values and start the API:

```bash
cd backend
APP_ENV=dev DATABASE_URL="postgres://sistemaemgo:sistemaemgo@localhost:5433/sistemaemgo?sslmode=disable" REDIS_ADDR=localhost:6379 JWT_SECRET="local-validation-secret-32-characters" go run ./cmd/api
```

Development seed validation:

```bash
docker compose up seed
```

The demo seed is intentionally guarded and should run only through the local Docker Compose flow or with `psql -v ALLOW_DEMO_SEED=1`. Do not run `backend/seed/seed.sql` in production.

## Production seed

Use `backend/seed/seed.prod.sql` only for production-safe reference data. It does not create default users or passwords.

Create the first production admin through a controlled bootstrap procedure:

1. Generate a strong temporary password outside source control.
2. Generate a bcrypt hash using an approved operational tool.
3. Insert the user, tenant membership, and tenant-scoped admin role in a controlled maintenance window.
4. Force password change at first login when that product flow exists.
5. Record the bootstrap in an operational audit ticket without storing the password.

## Backup

PostgreSQL logical backup example:

```bash
pg_dump "$DATABASE_URL" --format=custom --file=sistemaemgo-$(date +%Y%m%d%H%M).dump
```

PostgreSQL restore validation example:

```bash
createdb sistemaemgo_restore_test
pg_restore --dbname=sistemaemgo_restore_test --clean --if-exists sistemaemgo-YYYYMMDDHHMM.dump
```

Verify restored schema and application startup:

```bash
psql "$RESTORE_DATABASE_URL" -c "SELECT version, dirty FROM schema_migrations;"
psql "$RESTORE_DATABASE_URL" -c "SELECT COUNT(*) FROM companies;"
APP_ENV=staging FISCAL_PROVIDER=disabled DATABASE_URL="$RESTORE_DATABASE_URL" REDIS_URL="$STAGING_REDIS_URL" JWT_SECRET="$STAGING_JWT_SECRET" CORS_ALLOWED_ORIGINS="$STAGING_CORS_ALLOWED_ORIGINS" METRICS_BEARER_TOKEN="$STAGING_METRICS_BEARER_TOKEN" go run ./backend/cmd/api
```

After startup, run the restore subset in `docs/smoke-test.md`: login, refresh, product listing, sale creation/cancellation, fiscal listing/download, finance listing, audit listing, privacy request status/export, consent revocation, offline queue flush, and tenant isolation.

Redis stores refresh-token rotation/session state and optional cache data. Treat Redis availability and verified TLS as security-critical in staging/production. If Redis persistence is enabled, align it with the infrastructure recovery plan; otherwise, be prepared to force user reauthentication after Redis loss.

These backup/restore commands are operational procedures. Do not mark backup/restore as tested until the exact command, date, artifact, environment, and result are recorded in the release evidence.

## Migration rollback

- Back up PostgreSQL before migration windows.
- Review every new `*.down.sql` before release.
- Prefer forward fixes for already-public production migrations unless rollback is explicitly tested.
- Validate recent down/up migrations in staging before production.
- Keep application rollback and database rollback decisions coordinated; never roll back application code across incompatible schema changes without a tested plan.

## Deploy rollback

1. Stop traffic or route to the previous healthy version.
2. Confirm whether the database schema is backward compatible.
3. If schema rollback is required, restore backup or run tested down migrations.
4. Revoke compromised sessions if rollback is security-related.
5. Preserve logs, audit rows, request IDs, deployment SHAs, and migration versions for incident review.

## Configuration rollback

1. Keep each release's effective non-secret configuration snapshot and secret version references in the deployment record.
2. Roll back configuration by redeploying the previous known-good config and secret versions through the normal deployment system.
3. Never paste secret values into tickets, logs, or chat; reference secret-manager version IDs instead.
4. After rollback, verify `/health`, `/metrics` authentication, login, refresh, tenant-scoped RBAC, and one write flow in staging or the affected environment.
5. If JWT, Redis, or cookie settings changed, force reauthentication as needed and record the session impact.

## Restore drill

Run a restore drill in staging at least quarterly and before major fiscal/privacy releases:

- Restore the latest production-like backup into an isolated database.
- Apply pending migrations.
- Start the app against the restored database.
- Run the checklist in `docs/smoke-test.md`, at minimum login, refresh, product listing, sale creation/cancellation, fiscal XML listing/download, finance listing, audit logs, privacy request status/export, consent revocation, offline queue flush, and tenant isolation checks.
- Document duration, failures, and corrective actions.

## Production configuration checklist

- `APP_ENV` is `staging` or `prod`.
- `BUSINESS_TIMEZONE` is an explicit valid IANA timezone for the stores' reporting day (default `America/Sao_Paulo`).
- `JWT_SECRET` is strong, unique, and not a placeholder.
- `DATABASE_URL` uses a strong password, `sslmode=verify-full`, and a trusted CA/root certificate.
- `REDIS_URL` uses `rediss://` with a non-placeholder password and certificate verification. Split Redis settings are not accepted in staging/production.
- `CORS_ALLOWED_ORIGINS` contains only explicit trusted HTTPS origins.
- Metrics authentication is configured.
- `ALLOW_DEMO_SEED` is not enabled.
- Access token TTL is 15 minutes or less.
- Refresh cookie settings are compatible with HTTPS deployment.
- Tenant memberships and `user_tenant_roles` are explicitly provisioned.
- `FISCAL_PROVIDER=disabled` while no SEFAZ-ready provider is configured. The MVP provider is rejected in staging/production.
- Before migration `0013`, verify there is at most one `open` cash session per `(tenant_id, cash_register_id)`; duplicate rows must be reconciled explicitly. CI validates clean `v12 → v13`, `v13 → v12 → v13`, and rejection of legacy duplicate-open rows.
- Backup and restore drill has been completed in staging.


## Fiscal production guard

The built-in `mvp` provider generates only demonstration XML and is not SEFAZ-ready. Configuration validation rejects `FISCAL_PROVIDER=mvp` for staging/production. Use `FISCAL_PROVIDER=disabled` until a homologated provider with certificate signing, SEFAZ transmission/protocol handling, tax fields, numbering rules and DANFE is implemented and validated.

## Session revocation and shared terminals

- Logout is complete only after the backend confirms refresh-token revocation and clears the HttpOnly cookie. If the network is unavailable, keep the authenticated UI/session state and show the operator that logout was not completed; do not claim a local-only logout.
- Access and refresh token validation recheck both active-user status and current `user_tenants` membership. Removing a user from a tenant invalidates subsequent protected requests and refreshes for that tenant.
