# Production Validation Report

## Revalidation — 2026-09-23 (round 6 hardening)

- Base audited: `main` merge commit `646312aaa8b60be63906d21ffb29a3a14b6c213f`.
- Hardening branch / PR: `audit/e2e-hardening-round6-20260923` / PR #8.
- Last fully executed green implementation run: GitHub Actions `35808100170` on commit `8f07a6b841f4ce341ebbbcef8433d8cb38af5916`.
- That run passed backend, frontend, integration, security, e2e and e2e-prodlike.
- Browser E2E on that run: 23 discovered, 22 passed, 1 production-like-only skipped. Production-like E2E: 1/1 passed.
- The later branch head additionally hardens the browser offline queue: the server retains idempotency results for 30 days, while manual browser retry/rebind is capped at 28 days from the original intent timestamp. Older items remain visible for reconciliation but cannot emit a sale request. Integration coverage also verifies startup retention cleanup of a 31-day key while preserving a recent key.
- Subsequent attempts after the last fully green implementation run have ended as GitHub Actions startup failures with `steps: []`; those later retention/test/documentation changes have therefore not yet been executed by GitHub Actions.
- Cash operations now serialize through the session row, close waits for in-flight sale work, and cancel after close is rejected.
- `0015` adds supply/withdrawal and per-method reconciliation. Supply/withdrawal also write finance ledger entries atomically.
- Critical sale/cash audit events are transactional.
- Fiscal XML generation locks the sale and a fiscalized sale cannot be normally cancelled.
- Redis limiter repairs no-TTL legacy keys.
- Idempotency retention is bounded to 30 days and `0016` adds a retention index. Browser manual replay/rebind is bounded to 28 days so the server-side key cannot expire immediately before a client retry.

### Round 6 verdict

**The sixth-audit code findings are implemented, and the production-code SHA was fully green.** A same-HEAD rerun of the later test-only stabilization commits remains pending because GitHub Actions is currently returning runner startup failures before any step executes. These startup failures are infrastructure evidence, not repository test failures.
## Revalidation — 2026-09-22 (round 5 hardening)

- Base audited: `main` after merge commit `654a9eb53b12b4a8a30f432de49d64e2e0b9b88e`.
- Hardening branch: `audit/e2e-hardening-round5-20260922`.
- Implementation evidence run: GitHub Actions `35792634950` on commit `4a92f66b58c1566d1e1787b95f22ad031ce9c1ec`.
- CI result: backend PASS, frontend PASS, integration PASS, security PASS, e2e PASS, e2e-prodlike PASS.
- Browser E2E: 21 tests discovered; 20 passed and 1 production-like-only test was intentionally skipped in the normal job.
- Production-like E2E: 1/1 passed with built frontend over HTTPS, PostgreSQL `sslmode=verify-full`, Redis `rediss://`, secure refresh-cookie behavior and invalid-cookie cleanup.
- Sale creation requires `Idempotency-Key` at the backend boundary. The PDV writes the exact sale intent to its scoped offline queue before the first network send; storage failure blocks the send, eliminating the commit/response-loss/local-storage-loss ambiguity.
- Logout is authoritative: refresh-token revocation failure returns `503`, does not clear the cookie, and is audited as failure. Only successful server-side revocation can confirm logout.
- Invalid/expired refresh attempts expire the stale browser refresh cookie.
- Redis rate limits use an atomic Lua increment+TTL operation. Production-like middleware fails closed on Redis limiter errors; real Redis integration tests verify positive TTL and limit behavior.
- RBAC permissions are read from the current tenant-scoped repository for every protected request, removing the prior 30-second stale-permission window.
- Migration `0014_cash_reconciliation` adds expected/difference fields. Cash close computes expected physical cash from opening amount plus cash payments of finalized sales, persists the declared difference, returns it to the PDV and records `cash.open`/`cash.close` audit events.
- CI validates migration `0014` upgrade/rollback/reapply after the existing `0013` upgrade/duplicate-data checks.
- `govulncheck` reported 0 vulnerabilities reachable by the Go code; the production npm audit reported 0 vulnerabilities. The npm install still reports 9 development-ecosystem vulnerabilities (2 low, 1 moderate, 6 high), outside the production dependency gate.

### Round 5 verdict

**All code/CI findings from the fifth E2E audit are closed by automated evidence.** Remaining prerequisites are operational/environmental: production backup/restore evidence, load/failure-mode exercises, monitoring/alerting, least-privilege infrastructure credentials, legal/accounting/LGPD approval, and a SEFAZ-ready fiscal provider before real NF-e production use.

## Revalidation — 2026-09-22 (round 4 hardening)

- Base audited: `main` after merge commit `768fe7ffffe7944a3bdde256c05d4fdc857edfc9`.
- Hardening branch: `audit/e2e-hardening-round4-20260922`.
- Evidence run: GitHub Actions `35765112514` on commit `65c71269eeb64a15a80db9f9cdfdc688e1bd05b1`.
- CI result: backend PASS, frontend PASS, integration PASS, security PASS, e2e PASS, e2e-prodlike PASS.
- Browser E2E: 18 tests discovered; 17 passed and 1 production-like-only test was intentionally skipped in the normal job.
- Production-like E2E: 1/1 passed with built frontend over HTTPS, PostgreSQL `sslmode=verify-full` against a trusted CI CA, Redis over `rediss://`, secure HttpOnly refresh-cookie behavior, and server-confirmed logout semantics.
- PDV finalization now uses a synchronous in-flight guard, so rapid/double clicks cannot create two independent idempotency keys/requests.
- Rebinding an offline/legacy sale to the current cash session preserves the original `Idempotency-Key`; if the original request already committed, the backend conflict guard prevents a duplicate.
- Logout no longer clears local auth/state when the revocation request fails. Because the refresh cookie is HttpOnly, logout is considered complete only after the server confirms revocation and cookie expiration.
- Access-token validation and refresh rotation recheck current user↔tenant membership, so removing a user from a tenant invalidates existing tenant-scoped sessions immediately.
- Staging/production configuration now requires PostgreSQL `sslmode=verify-full` and Redis `rediss://`; split Redis settings remain dev/legacy-only.
- Integration CI validates migration `v12 → v13`, `v13 → v12` rollback, explicit rejection of seeded legacy duplicate-open cash sessions, and successful reapplication after reconciliation.
- `govulncheck` reported 0 vulnerabilities reachable by the Go code; the production npm audit reported 0 vulnerabilities.

### Round 4 verdict

**All code/CI findings from the fourth E2E audit are closed by automated evidence.** The remaining prerequisites are operational/environmental rather than known code defects: production backup/restore evidence, load/failure-mode exercises, monitoring/alerting, least-privilege infrastructure credentials, legal/accounting/LGPD approval, and a SEFAZ-ready fiscal provider before real NF-e production use.

## Revalidation — 2026-09-22 (round 3 hardening)

- Base audited: `main` after merge commit `38bb1dd2f325396ebde016f60eb71ff8125fe7b8`.
- Hardening branch: `audit/e2e-hardening-round3-20260922`.
- Implementation evidence run: GitHub Actions `35759350936` on commit `e52b90333f09e8f60a7de4fe5f27de2b3e035fdd`.
- CI result: backend PASS, frontend PASS, integration PASS, e2e PASS, e2e-prodlike PASS, security PASS.
- Browser E2E: 14 tests passed and 1 production-like-only test was intentionally skipped in the normal job. The complete stateful functional suite runs deterministically in Chromium; Firefox and WebKit run an authenticated UI/PDV smoke to validate cross-engine compatibility without competing for shared transactional test data.
- Production-like E2E: 1/1 passed through an HTTPS reverse proxy with a built frontend, staging configuration, PostgreSQL TLS, password-protected Redis, trusted proxy configuration, and secure refresh-cookie attributes.
- Cash lifecycle is closed end to end: the UI closes the backend session, duplicate open/close operations return conflicts, and migration `0013` enforces at most one open session per tenant/register. Existing duplicate sessions make the migration fail explicitly for manual reconciliation rather than being silently altered.
- Legacy offline queue `v1` is detected but never auto-attributed to a tenant. An authenticated operator can explicitly import legacy items into attention/reconciliation state, bind a sale to the current cash session with a new idempotency key, retry, or discard it.
- Attention-state offline items are visible and individually actionable in the PDV instead of becoming an operational dead end.
- A definitive refresh failure now propagates auth-state invalidation to the React route guard and immediately redirects to login.
- Real two-tenant browser E2E verifies that tenant B cannot retrieve tenant A sale, fiscal XML, finance ledger references, privacy requests, or audit resource IDs.
- `FISCAL_PROVIDER=mvp` is rejected in staging/production. Production-like configuration uses `FISCAL_PROVIDER=disabled` until a SEFAZ-ready provider is implemented and homologated.
- Production dependency audit and `govulncheck` remain green.

### Round 3 verdict

**Code and CI findings from the third E2E audit are closed.** The repository is a strong production-like staging candidate. This is not approval for Brazilian fiscal production: real NF-e issuance remains intentionally disabled until a SEFAZ-ready provider (certificate/signature, tax fields, transmission/protocol handling and DANFE as applicable) is implemented and validated. Environment-specific backup/restore, load/failure-mode, monitoring, least-privilege credentials and legal/accounting/LGPD approvals remain operational prerequisites.

## Revalidation — 2026-09-22

- Base audited: `main` after merge commit `7a5cfaa7216554de9f9f15eee9938f187dfc1d0e`.
- Hardening branch: `audit/e2e-offline-integrity-20260922`.
- Evidence run: GitHub Actions `35690823333` on commit `036a7e7447d45e6cc349f12ffd0751fb2587a521`.
- CI result: backend PASS, frontend PASS, integration PASS, e2e PASS, security PASS.
- Browser E2E: 7 tests passed in Chromium.
- Tagged PostgreSQL integration test now runs in CI with `go test -tags=integration ./tests/integration/...` and validates cross-tenant product isolation.
- Offline PDV validates normal offline/reconnect plus ambiguous response loss: a retry reuses the original `Idempotency-Key` and the backend records only one sale.
- Browser operational state (cash session, product cache, offline queue) is scoped by authenticated tenant + user.
- Offline items older than 24 hours are preserved in an attention state rather than silently deleted.
- Permanent 4xx reconciliation failures are preserved for attention and no longer block later queued sales.
- Production dependency audit passed with 0 production vulnerabilities; `govulncheck` reported 0 vulnerabilities reachable by the Go code.

### Updated staging verdict

**Ready for production-like staging validation.** The code/CI blockers identified in the May 2026 report and the September E2E audit are closed by automated evidence above. This still is not a production approval: real secrets/TLS/reverse proxy behavior, least-privilege production DB/Redis configuration, staging backup/restore drill, monitoring/load/failure-mode exercises, and legal/accounting/DPO approvals remain environment/operational prerequisites.

### Superseded statements in the original report

The original 2026-05-03 report below is retained as historical evidence. Its statements that browser offline POS testing was skipped and that GitHub CI still needed to run are no longer current; both now have automated GitHub Actions evidence in the 2026-09-22 revalidation above.

## Original validation — 2026-05-03

## Executive Summary

- Overall verdict: Ready for staging
- Date: 2026-05-03
- Environment: Windows PowerShell, Docker Compose v5.1.2, PostgreSQL 16 container, Redis 7.4 container, local Go 1.26.2, Node.js v24.11.0, npm 11.6.1
- Validator: Codex
- Repository: https://github.com/GustavoMFDS/SistemaERP
- Branch: production-readiness-validation
- Commit before validation: 78655c806fb76e729bd289b5f6e3a2cf7001d899
- Commit after validation: recorded in Git after this report is committed; a commit cannot contain its own final SHA without changing that SHA

The validation found one real first-run bug: opening the first cash session on a clean database failed because the default cash register creation query did not read from the inserted CTE result. The query was fixed and the clean migration, seed, backend, frontend, API smoke, backup/restore, and rollback checks were rerun successfully. The only skipped production-readiness item is browser-level offline POS queue simulation.

## Validation Matrix

| Area | Status | Notes |
|---|---|---|
| Git safety check | PASS | Remote points to `https://github.com/GustavoMFDS/SistemaERP.git`; branch is `production-readiness-validation`; `.env` is not tracked. |
| Docker Compose services | PASS | Services are `db`, `redis`, `migrate`, `seed`; `db` and `redis` started healthy with ports `5433:5432` and `6379:6379`. |
| Frontend clean install | PASS | `npm.cmd ci` added/audited 273 packages and reported 0 vulnerabilities. |
| Frontend lint | PASS | `npm.cmd run lint` completed successfully. |
| Frontend build | PASS | `npm.cmd run build` completed; Vite generated `dist/index.html`, CSS, and JS assets. |
| Backend tests | PASS | `go test ./...` passed. Local Go was 1.26.2; CI is configured for Go `1.25.x`. |
| Backend vet | PASS | `go vet ./...` passed. |
| Backend gofmt | PASS | `gofmt -l .` returned no files. |
| Migrations clean DB | PASS | `docker compose down -v` then `docker compose up -d db redis migrate`; migrations reached version 12, dirty false. |
| Idempotency immutability | PASS | Direct SQL update attempt was rejected by `idempotency_keys_immutable`. |
| Seed safety | PASS | Production seed created no users; demo seed failed without `ALLOW_DEMO_SEED=1`; explicit dev seed created demo users/data. |
| Backend startup | PASS | API started against clean migrated DB and restored DB; `/health` returned HTTP 200. |
| Smoke tests | WARNING | API smoke passed for auth, RBAC, tenancy, sales, inventory, finance, fiscal, audit, privacy, consent; offline browser queue simulation was skipped. |
| Backup/restore | PASS | `pg_dump` and `pg_restore` into `sistemaemgo_restore_validation` passed; schema/data and app startup/login verified. |
| Rollback readiness | PASS | Deployment docs cover app, migration, config rollback and restore; recent migration `0012` down/up validated. |
| Security checklist | PASS | Checklist passed; see detailed section. |

## Commands Executed

Git safety:

```bash
git remote -v
git branch --show-current
git rev-parse HEAD
git status --short
git ls-files .env
git checkout -b production-readiness-validation
```

Docker and migrations:

```bash
docker compose config --services
docker compose down -v
docker compose up -d db redis migrate
docker compose ps
docker compose logs db redis migrate --tail=120
docker compose exec -T db psql -U sistemaemgo -d sistemaemgo -c "SELECT version, dirty FROM schema_migrations;"
```

Schema checks:

```bash
docker compose exec -T db psql -U sistemaemgo -d sistemaemgo -c "SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('companies','users','roles','permissions','user_tenant_roles','idempotency_keys','audit_logs','data_subject_requests','consent_records') ORDER BY table_name;"
docker compose exec -T db psql -U sistemaemgo -d sistemaemgo -c "SELECT column_name, is_nullable FROM information_schema.columns WHERE table_name='idempotency_keys' AND column_name IN ('tenant_id','operation','idem_key','request_hash') ORDER BY column_name;"
docker compose exec -T db psql -U sistemaemgo -d sistemaemgo -c "SELECT tgname FROM pg_trigger WHERE tgrelid='idempotency_keys'::regclass AND NOT tgisinternal ORDER BY tgname;"
docker compose exec -T db psql -U sistemaemgo -d sistemaemgo -c "SELECT code FROM permissions WHERE code IN ('privacy:read','privacy:write','audit:read') ORDER BY code;"
docker compose exec -T db psql -U sistemaemgo -d sistemaemgo -c "SELECT indexname FROM pg_indexes WHERE schemaname='public' AND indexname IN ('idempotency_keys_tenant_op_key_hash_idx','audit_logs_tenant_created_idx','audit_logs_tenant_action_created_idx','audit_logs_tenant_resource_created_idx','audit_logs_tenant_actor_created_idx','audit_logs_tenant_outcome_created_idx','data_subject_requests_tenant_status_idx','consent_records_tenant_subject_idx','user_tenant_roles_tenant_user_idx','sales_tenant_status_created_idx','products_tenant_active_created_idx') ORDER BY indexname;"
```

Idempotency immutability:

```bash
docker compose exec -T db psql -U sistemaemgo -d sistemaemgo -v ON_ERROR_STOP=1 -c "DO $$ ... attempt idempotency_keys request_hash update ... $$;"
```

Result: update rejected by the immutability trigger and the temporary row was removed.

Seed validation:

```bash
docker compose run --rm -e PGPASSWORD=sistemaemgo -v C:\Projetos\SistemaEmGo\backend\seed:/seed:ro db psql -h db -U sistemaemgo -d sistemaemgo -v ON_ERROR_STOP=1 -f /seed/seed.prod.sql
docker compose run --rm -e PGPASSWORD=sistemaemgo -v C:\Projetos\SistemaEmGo\backend\seed:/seed:ro db psql -h db -U sistemaemgo -d sistemaemgo -v ON_ERROR_STOP=1 -f /seed/seed.sql
docker compose run --rm -e PGPASSWORD=sistemaemgo -v C:\Projetos\SistemaEmGo\backend\seed:/seed:ro db psql -h db -U sistemaemgo -d sistemaemgo -v ON_ERROR_STOP=1 -v ALLOW_DEMO_SEED=1 -f /seed/seed.sql
```

Expected intentional result: unflagged demo seed failed safely with `ERROR: division by zero`; explicit dev seed succeeded.

Backend and frontend validation:

```bash
cd backend
gofmt -l .
go test ./...
go vet ./...
go build -o tmp\api-validation.exe .\cmd\api

cd web
npm.cmd ci
npm.cmd run lint
npm.cmd run build
```

Backend startup:

```powershell
APP_ENV=dev
DATABASE_URL=postgres://sistemaemgo:sistemaemgo@localhost:5433/sistemaemgo?sslmode=disable
REDIS_ADDR=localhost:6379
JWT_SECRET=local-validation-secret-32-characters
HTTP_ADDR=:18080
backend\tmp\api-validation.exe
Invoke-WebRequest http://localhost:18080/health
Invoke-WebRequest http://localhost:18080/metrics
```

Result: `/health` returned `200 {"status":"ok"}`; `/metrics` returned HTTP 200 in dev mode.

Backup/restore:

```bash
docker compose exec -T db pg_dump -U sistemaemgo -d sistemaemgo --format=custom --file=/tmp/sistemaemgo-validation.dump
docker compose exec -T db psql -U sistemaemgo -d postgres -v ON_ERROR_STOP=1 -c "DROP DATABASE IF EXISTS sistemaemgo_restore_validation WITH (FORCE);" -c "CREATE DATABASE sistemaemgo_restore_validation OWNER sistemaemgo;"
docker compose exec -T db pg_restore -U sistemaemgo --dbname=sistemaemgo_restore_validation --clean --if-exists /tmp/sistemaemgo-validation.dump
docker compose exec -T db psql -U sistemaemgo -d sistemaemgo_restore_validation -c "SELECT version, dirty FROM schema_migrations; SELECT COUNT(*) AS users_count FROM users; SELECT COUNT(*) AS products_count FROM products; SELECT COUNT(*) AS sales_count FROM sales; SELECT COUNT(*) AS audit_count FROM audit_logs;"
```

Restore verification returned migration version 12 dirty false, 3 users, 2 products, 1 sale, and 18 audit rows. API startup and login against the restored DB returned HTTP 200.

Rollback validation:

```bash
docker compose run --rm migrate -path=/migrations -database=postgres://sistemaemgo:sistemaemgo@db:5432/sistemaemgo?sslmode=disable down 1
docker compose run --rm migrate -path=/migrations -database=postgres://sistemaemgo:sistemaemgo@db:5432/sistemaemgo?sslmode=disable up 1
```

Result: `12/d tenant_scoped_roles` and `12/u tenant_scoped_roles` succeeded; migration status returned version 12 dirty false.

## Smoke Tests

| Test | Expected | Actual | Status |
|---|---|---|---|
| Backend health endpoint | HTTP 200 | HTTP 200 | PASS |
| Metrics endpoint in dev | HTTP 200 | HTTP 200, metrics returned | PASS |
| Protected route rejects unauthenticated request | HTTP 401 | HTTP 401 | PASS |
| Login fails with invalid credentials | HTTP 401 | HTTP 401 | PASS |
| Login succeeds with valid admin user | HTTP 200, token present | HTTP 200, token present | PASS |
| Refresh session works | HTTP 200, rotated token | HTTP 200, token present | PASS |
| Logout works | HTTP 200 | HTTP 200 | PASS |
| List products | HTTP 200 with seeded product | HTTP 200 | PASS |
| Tenant A cannot access tenant B product | HTTP 404/not found | HTTP 404 | PASS |
| Stock adjustment works | HTTP 200 | HTTP 200 | PASS |
| Inventory movement recorded | HTTP 200 with movement | HTTP 200, total 1 | PASS |
| Open first cash session on clean DB | HTTP 201 | HTTP 201 | PASS |
| Create sale and backend price calculation | HTTP 201, total 10.90 despite tampered `unit_price` | HTTP 201, total 10.90 | PASS |
| Idempotent replay returns previous result | HTTP 200 replayed true same id | HTTP 200 replayed true | PASS |
| Idempotent replay with different payload conflicts | HTTP 409 | HTTP 409 | PASS |
| Insufficient stock rejected | HTTP 409 | HTTP 409 | PASS |
| Fiscal mock/provider generation works | HTTP 201 XML id | HTTP 201 | PASS |
| Fiscal XML download works for authorized user | HTTP 200 XML | HTTP 200 XML | PASS |
| Fiscal download is RBAC-protected | HTTP 403 for cashier | HTTP 403 | PASS |
| Finance ledger listing works | HTTP 200 with ledger rows | HTTP 200, total 1 | PASS |
| Cancel sale restores stock through service flow | HTTP 200 | HTTP 200 | PASS |
| Create privacy request | HTTP 201 | HTTP 201 | PASS |
| List privacy requests | HTTP 200 | HTTP 200 | PASS |
| Valid privacy status transition works | open -> in_progress -> completed | HTTP 200/200 | PASS |
| Terminal privacy status transition is rejected | HTTP 409 | HTTP 409 | PASS |
| Privacy export subject data works | HTTP 200 | HTTP 200 | PASS |
| Privacy anonymize works where applicable | HTTP 200 | HTTP 200 | PASS |
| Consent creation works | HTTP 201 | HTTP 201 | PASS |
| Consent revocation works | HTTP 200 | HTTP 200 | PASS |
| Audit listing requires audit:read and returns sanitized metadata | HTTP 200, no obvious secret strings | HTTP 200 | PASS |
| Unauthorized action fails through RBAC | HTTP 403 for cashier audit logs | HTTP 403 | PASS |
| Offline POS queue browser simulation | Browser offline/online local queue simulation | Not executed in API-only smoke pass | SKIPPED |

## Security Checklist

| Item | Status | Evidence |
|---|---|---|
| `.env` is not tracked | PASS | `git ls-files .env` returned no files. |
| `.env` excluded from source package/archive | PASS | `.gitattributes` sets `export-ignore` for `.env` and `.env.*`; safe examples are explicitly kept. |
| `.env.example` and `.env.prod.example` contain placeholders only | PASS | Reviewed examples; production placeholders use `REPLACE_WITH...` values and docs warn to replace them. |
| Production config rejects weak JWT secret | PASS | `backend/internal/config/config_test.go` covers long TTL and placeholder secret rejection. |
| Production config rejects missing/disabled Redis when required | PASS | Config validation rejects `DISABLE_REDIS=true` in staging/prod; app startup also fails if Redis is disabled/unavailable. |
| Production config rejects insecure Redis URL/passwordless Redis | PASS | `REDIS_URL` validation requires `redis`/`rediss` and a non-placeholder password in prod-like environments. |
| Production config rejects missing CORS allowed origins | PASS | Config validation requires explicit origins in staging/prod. |
| Production config rejects unprotected metrics | PASS | Config validation requires bearer token or basic auth credentials in staging/prod. |
| Refresh token is cookie-only | PASS | Public auth response omits refresh token; refresh reads `__Host-refresh_token` cookie. |
| Access token is not stored in localStorage/sessionStorage | PASS | `web/src/lib/auth.ts` keeps access token in memory and removes legacy `auth_token` storage keys. |
| Audit sanitizer redacts secrets | PASS | Unit tests cover password, tokens, cookie, authorization, API key, structs, typed maps, arrays, and oversized metadata. |
| Rate limit keys do not expose raw emails | PASS | Login identifier keys use SHA-256 normalized identifier hashes. |
| Tenant fallback disabled in staging/prod | PASS | `modules.New` only enables fallback when `!cfg.IsProdLike()`; repository returns forbidden otherwise. |
| Demo seed cannot run accidentally in production | PASS | Startup config rejects `ALLOW_DEMO_SEED` in prod-like envs; seed SQL fails without explicit flag. |
| Metrics endpoint protected in staging/prod | PASS | Middleware/config require metrics auth outside dev; dev mode metrics intentionally returned HTTP 200. |
| CORS does not allow wildcard with credentials | PASS | Config validation rejects wildcard origins; CORS only echoes configured origins. |

## Passed Tests

- Git remote, branch, status, and `.env` tracking checks.
- Docker Compose `db` and `redis` startup and health checks.
- Clean migrations from zero through version 12.
- Schema checks for base tables, tenants/companies, users, roles/permissions, tenant-scoped roles, idempotency, audit, privacy, consent, permissions, and indexes.
- Idempotency immutability trigger validation.
- Production seed safety and development seed explicit flag behavior.
- Backend `gofmt`, tests, vet, build, startup, health, and dev metrics.
- Frontend clean install, lint, and production build.
- API smoke tests listed above.
- PostgreSQL backup/restore drill and app startup/login against restored DB.
- Recent migration down/up rollback validation.
- Security checklist.

## Failed Tests

No unresolved failed tests remain.

During the first smoke run, opening the first cash session on a clean database failed. Likely cause was a PostgreSQL statement snapshot issue in `CashRepo.EnsureDefaultRegister`: the query inserted the default register in a CTE but selected from `cash_registers`, which did not see the just-inserted row in the same statement. Fix: select from the CTE result unioned with existing registers. After the fix, backend tests/vet passed and the full clean DB/API smoke path passed.

## Skipped Tests

- Offline POS queue browser simulation: SKIPPED because this validation pass used API and direct database checks, not a browser with offline/online network toggling. Required to run: browser automation or manual UI test that disables network, enqueues a pending sale, verifies idempotency key storage, flushes after reconnect, verifies no duplicate sale, and confirms logout behavior with pending items.

## Warnings

- Local Go version is `go1.26.2`; project `go.mod` and GitHub Actions are configured for Go `1.25.x`. Local validation passed, but CI on Go `1.25.x` remains the exact configured-version source of truth.
- Docker emitted local config access warnings for `C:\Users\gusta\.docker\config.json`; Compose operations still succeeded after approval.
- The local PostgreSQL container logs note trust authentication for local container initialization. This is Docker-local validation behavior and not a production database configuration.
- Dev seed intentionally creates known demo users/passwords only when explicitly flagged. Do not run it outside dev/test.

## Remaining Risks

- Browser-level offline POS queue behavior still needs manual or browser-automated validation.
- Production secrets, TLS termination, database roles, Redis authentication/TLS, and infrastructure network policy must be validated in the real staging/production environment.
- Legal, accounting, DPO, and retention-policy approval remain required before production use.
- CI should be run on GitHub to validate the exact Go `1.25.x` and Node CI environment.
- Load/performance and failure-mode tests beyond smoke coverage are still recommended before real production.

## Final Verdict

Ready for staging.

This is not a full production approval. It is suitable for a production-like staging deployment and controlled operational validation. Do not promote to real production until the skipped offline queue simulation, GitHub CI, staging restore drill evidence, secrets/infrastructure review, and legal/accounting/DPO sign-off are complete.

## Next Steps

1. Run GitHub Actions on the pushed branch and confirm Go `1.25.x` CI passes.
2. Execute browser/manual offline POS queue smoke testing.
3. Run this backup/restore drill in staging with production-like infrastructure and record artifact IDs/duration.
4. Validate production secret-manager values against `.env.prod.example`.
5. Review TLS, Redis auth/TLS, database least-privilege roles, log retention, and monitoring alerts.
6. Complete legal/accounting/DPO review for LGPD and fiscal retention procedures.
