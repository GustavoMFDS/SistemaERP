# SistemaEmGo

SistemaEmGo is a multi-tenant ERP/POS and backoffice system for sales, inventory, finance, fiscal operations, auditability, and privacy/LGPD technical workflows.

The project is currently a pre-production / staging candidate. It implements technical controls that support LGPD compliance, but final legal, accounting, DPO, infrastructure, and operational validation is required before real production use.

## Features

- Multi-tenant POS/backoffice with tenant-scoped RBAC.
- Sales creation, server-side price calculation, barcode lookup/scanner support, idempotent offline retry, discounts with RBAC, suspended carts, and sale cancellation.
- Inventory balances, movements, low-stock checks, stock validation, and dedicated purchase/return movement provenance.
- Supplier and procurement workflows with tenant-scoped RBAC, partial receiving, stock/cost updates on receipt, and optional accounts payable linkage.
- Returns/exchanges with quantity guards, optional restock, refund-due calculation, and cancellation protection after a return.
- Finance ledger/dashboard, digital payment reconciliation, partial/multimethod refund settlement, and per-method cash-session closing.
- Fiscal XML/NFe preparation and access flows. The bundled provider is an MVP test provider and is blocked in staging/production.
- Offline POS queue with TTL, idempotency keys, and minimal browser storage.
- JWT access tokens plus HttpOnly refresh-token cookies with rotation.
- Standardized JSON API errors with request IDs.
- Audit logging with sanitized metadata.
- Privacy/LGPD technical workflows for data subject requests, exports, anonymization/blocking, and consent records.
- Observability with health checks, structured logs, tracing hooks, and protected Prometheus metrics.

## Tech Stack

- Go `1.25.0` for the backend.
- React 19, TypeScript, and Vite for the frontend.
- PostgreSQL for persistent data.
- Redis for refresh-token rotation, rate limiting, and optional caches.
- Docker Compose for local dependencies, migrations, and development seed.
- GitHub Actions CI for backend and frontend validation.

## Repository Structure

- `backend/`: Go API, modules, migrations, seed files, and backend tests.
- `web/`: React/Vite frontend.
- `docs/`: API, security, privacy, deployment, and smoke-test documentation.
- `.github/workflows/ci.yml`: CI validation using `go test`, `go vet`, `gofmt`, `npm ci`, lint, and build.

## Prerequisites

- Go `1.25.x`.
- Node.js `22.x`.
- Docker and Docker Compose.
- PostgreSQL and Redis if running without Docker Compose.

## Quick Start For Development

```bash
cp .env.example .env
docker compose up -d db redis migrate
docker compose up seed
```

Run the backend:

```bash
cd backend
go run ./cmd/api
```

Run the frontend:

```bash
cd web
npm ci
npm run dev
```

The development seed is guarded by `ALLOW_DEMO_SEED=1` inside the Docker Compose seed service. Do not run the demo seed in staging or production.

## Environment Variables

`.env` is local-only. It is ignored by Git and Docker packaging rules and must never be committed, shipped in source archives, attached to tickets, or copied into logs. Keep only safe examples in source control: `.env.example`, `.env.dev.example`, and `.env.prod.example`.

Use `.env.example` for local development and `.env.prod.example` for staging/production. Replace every production placeholder with a real secret or environment-specific value before startup.

Important variables:

- `APP_ENV`: `dev`, `test`, `staging`, or `prod`.
- `DATABASE_URL`: PostgreSQL connection string; staging/production require `sslmode=verify-full`.
- `REDIS_URL`: required `rediss://` connection string for staging/production.
- `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB`: split Redis configuration for development/legacy deployments only.
- `JWT_SECRET`: strong signing secret, at least 32 characters.
- `ACCESS_TOKEN_TTL_MINUTES`, `REFRESH_TOKEN_TTL_MINUTES`: token lifetimes.
- `CORS_ALLOWED_ORIGINS`: explicit origins in staging/production.
- `TRUSTED_PROXY_CIDRS`: reverse-proxy networks allowed to supply forwarded client IP headers; leave empty for direct exposure.
- `METRICS_BEARER_TOKEN` or `METRICS_BASIC_USER` / `METRICS_BASIC_PASS`: required for metrics in staging/production.
- `RATE_LIMIT_*`: sensitive endpoint rate limits.
- `FISCAL_PROVIDER`: `mvp` only for dev/test; use `disabled` in staging/production until a SEFAZ-ready provider exists.
- `PRIVACY_CONTACT_EMAIL`, `APP_PUBLIC_URL`: privacy/DPO contact and public URL.

See [docs/security.md](docs/security.md), [.env.example](.env.example), and [.env.prod.example](.env.prod.example) for configuration details.

## Database Migrations

Apply migrations locally:

```bash
docker compose up -d db redis migrate
```

Validate migrations on a clean local database:

```bash
docker compose down -v
docker compose up -d db redis migrate
```

Production seed safety:

- `backend/seed/seed.prod.sql` contains production-safe reference data only.
- `backend/seed/seed.sql` is development/demo seed and requires explicit `ALLOW_DEMO_SEED=1`.
- Production bootstrap must create the first admin through a controlled operational procedure, not default credentials.

See [docs/deployment.md](docs/deployment.md) for clean migration validation, backup, restore, and rollback procedures.

## Running Tests And Validation

Backend:

```bash
cd backend
gofmt -l .
go test ./...
go vet ./...
```

Frontend:

```bash
cd web
npm ci
npm run lint
npm run build
```

A prior production-readiness pass validated the production build. The current retail-integration PR changes backend, frontend, migrations and E2E coverage, so its final SHA must rerun the complete GitHub Actions matrix before merge. See `docs/production-validation-report.md` for the current evidence status.

Smoke testing:

```bash
# Follow the checklist in docs/smoke-test.md after deploying to staging.
```

## Security Overview

- Access tokens are short-lived JWTs.
- Protected requests recheck current user status and tenant membership, so deactivated or de-scoped users are rejected before token expiration.
- Refresh tokens are stored only in an HttpOnly, `SameSite=Strict` cookie and rotated on refresh; logout is considered complete only after the server confirms revocation/cookie clearing.
- Refresh/logout endpoints validate trusted `Origin` or `Referer` headers.
- Login is rate-limited by IP, by hashed normalized identifier, and by IP plus identifier.
- RBAC is tenant-scoped through `user_tenant_roles`; a role in tenant A does not grant tenant B permissions.
- Tenant fallback to the first company is disabled in staging/production.
- `/metrics` requires bearer token or basic auth in staging/production.
- Audit metadata is recursively sanitized and size-limited before storage and API output.
- API errors are standardized JSON and avoid leaking stack traces, SQL details, secrets, or tokens.

See [docs/security.md](docs/security.md) for the detailed security model.

## Privacy / LGPD Technical Controls

The system provides technical controls for:

- Data subject request creation, listing, viewing, and status transitions.
- Personal data export for supported `customer` and `user` subjects.
- Anonymization/blocking where legally applicable.
- Consent recording, listing, and revocation.
- Audit logging for privacy operations.
- Retention and disposal documentation.

Terminal privacy request statuses (`completed`, `rejected`, `cancelled`) are immutable through the status endpoint. Fiscal/accounting records are not hard-deleted by privacy endpoints when legal retention applies.

See [docs/privacy/data-inventory.md](docs/privacy/data-inventory.md), [docs/privacy/retention-policy.md](docs/privacy/retention-policy.md), and [docs/privacy/ropa.md](docs/privacy/ropa.md).

## Observability

- `/health/live` checks process liveness; `/health/ready` verifies PostgreSQL and Redis readiness.
- Prometheus metrics are exposed through `/metrics` and protected outside development.
- Structured logs include request IDs.
- Audit logs support incident investigation without storing secrets or excessive personal data.

## Deployment

Use [docs/deployment.md](docs/deployment.md) for:

- Migration validation.
- Production configuration checklist.
- Backup and restore commands.
- Redis recovery considerations.
- Application and migration rollback strategy.
- Staging restore drill.

Use [docs/smoke-test.md](docs/smoke-test.md) for critical functional validation before promoting a release.

## Production Readiness Checklist

- CI is green for backend and frontend.
- Frontend production build has been validated with `npm run build`; run `npm ci`, lint, and build again after frontend dependency or source changes.
- Backend `gofmt`, tests, and vet pass with Go `1.25.x`.
- Clean database migrations have been validated.
- Production secrets are strong and not placeholders.
- CORS origins are explicit.
- Metrics are protected.
- Demo seed is disabled.
- Tenant memberships and tenant-scoped roles are configured.
- Backup/restore has been tested in staging.
- Smoke test checklist has passed.
- Legal/accounting/DPO review has approved retention and privacy procedures.

## License / Status

License: define before public distribution.

Status: pre-production / staging candidate.
