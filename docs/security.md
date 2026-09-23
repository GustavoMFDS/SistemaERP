# Security controls

Implemented or prepared controls:

- JWT access tokens are short-lived by configuration (`ACCESS_TOKEN_TTL_MINUTES`, example default 15 minutes; staging/production rejects values above 15).
- Protected requests and refresh-token rotation recheck current user status and current tenant membership. Deactivated users or users removed from the token tenant are rejected before token expiry.
- Refresh token rotation is backed by Redis and refresh tokens are issued as `HttpOnly`, `SameSite=Strict` cookies. Refresh tokens are not returned in JSON auth responses. Invalid/expired refresh attempts expire the browser cookie.
- Logout is considered complete only after Redis-backed refresh-token revocation succeeds and the server emits cookie expiration. Revocation failure returns `503` and does not clear the cookie or report success; a browser/network failure leaves the authenticated UI in place with an explicit retry error.
- Cookie-authenticated refresh/logout endpoints validate `Origin`/`Referer` against `CORS_ALLOWED_ORIGINS`; staging/production requires one of those headers.
- Sensitive endpoints use rate limiting. Login is limited by IP, normalized identifier, and IP plus identifier. Redis counters use an atomic Lua `INCR` + TTL operation. Staging/production fail closed with `503` if Redis limiting is unavailable; the local in-memory limiter is development/testing fallback only.
- Login identifier rate-limit keys are SHA-256 hashes of normalized identifiers; raw emails are not embedded in Redis/local limiter keys.
- Best-effort audit logging records auth, product, inventory, cash open/close, sale, fiscal XML, and privacy/consent actions with tenant when known, actor, request ID, IP, user agent, outcome, and minimal metadata. Cash close audit metadata includes expected, declared, and difference values.
- Audit metadata is sanitized recursively and size-limited before persistence. Sensitive keys such as password, token, cookie, authorization, secret, API key, credential, session, and JWT are redacted.
- RBAC roles are tenant-scoped through `user_tenant_roles`; `user_roles` is retained only as legacy source data for migration/backward compatibility. Effective permissions are read from the tenant-scoped repository on each protected request so role/permission revocation is immediate.
- Users must have explicit `user_tenants` membership in staging/production. The first-company tenant fallback is disabled outside development/test to avoid cross-tenant privilege surprises.
- Passwords use bcrypt hashes; raw passwords and tokens must never be logged.
- API errors use JSON with stable codes and `request_id`; internal details are not sent to clients.
- JSON bodies are size-limited, reject unknown fields, and reject multiple JSON values.
- CORS is environment-configurable and production requires explicit allowed origins.
- Security headers are applied globally, including CSP, content-type sniffing protection, referrer policy, frame protection, permissions policy, and HSTS in production-like environments.
- `/metrics` is public only in development by default. Staging/production must configure `METRICS_BEARER_TOKEN` or basic auth credentials.
- Redis can be disabled only in development. Staging/production require `rediss://`; PostgreSQL requires `sslmode=verify-full`. Startup fails when these verified-TLS requirements are not met.

Sensitive environment variables:

- `JWT_SECRET` or `JWT_SECRET_FILE`
- `DATABASE_URL` or `DB_PASSWORD_FILE`
- `REDIS_URL` using `rediss://` in staging/production; split Redis settings are development/legacy-only
- `METRICS_BEARER_TOKEN`
- `METRICS_BASIC_PASS`
- `ALLOW_DEMO_SEED` must not be enabled in staging/production.

Rate-limit environment variables:

- `RATE_LIMIT_LOGIN_PER_MINUTE`
- `RATE_LIMIT_LOGIN_IDENTIFIER_PER_MINUTE`
- `RATE_LIMIT_LOGIN_IP_IDENTIFIER_PER_MINUTE`
- `RATE_LIMIT_REFRESH_PER_MINUTE`
- `RATE_LIMIT_LOGOUT_PER_MINUTE`
- `RATE_LIMIT_SALES_PER_MINUTE`
- `RATE_LIMIT_FISCAL_PER_MINUTE`

Production notes:

- Terminate TLS at a trusted reverse proxy/load balancer and forward only HTTPS traffic to users.
- Use least-privilege database roles for the app and migrations separately.
- Keep tenant-scoped cache keys and queries; all high-risk queries must include `tenant_id`.
- Validate LGPD legal bases and retention periods with counsel before production launch.
- Demo seed data requires explicit `ALLOW_DEMO_SEED=1` in psql and is blocked by startup validation in staging/production.
- Production bootstrap must create the first administrator with an explicit tenant membership and tenant-scoped role assignment; default demo credentials are never created by the production seed.
- Validate backups and restores before production. Migration rollback should be a planned operational procedure, not improvised during an incident.
- See `docs/deployment.md` for backup, restore, migration validation, and rollback procedures.

## Round 6 security/integrity controls
- Critical `sale.create`, `sale.cancel`, `cash.open`, `cash.supply`, `cash.withdrawal` and `cash.close` audit rows are inserted in the same PostgreSQL transaction as the business mutation.
- Redis rate limiting repairs legacy keys that have no TTL by reapplying `PEXPIRE` when `PTTL < 0`.
- Cash-session `FOR UPDATE` serialization prevents sale/cancel/movement operations from racing past cash close.
- Normal cancellation is blocked once a fiscal invoice exists; a SEFAZ-ready fiscal cancellation flow is required before production fiscal cancellation can be supported.
- Idempotency retention runs on startup/hourly in bounded batches and is backed by the `idempotency_keys_created_at_idx` index.
