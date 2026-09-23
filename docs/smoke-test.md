# Production smoke test checklist

Use this checklist after deploying to staging, after restoring a backup, and before promoting a release. Record the build SHA, migration version, tester, timestamp, environment, and relevant request IDs.

## Preconditions

- Staging uses production-like `APP_ENV=staging`.
- Migrations have been applied from a clean schema or the expected release baseline.
- Redis is available if refresh-token rotation and rate limiting are enabled.
- Metrics protection is configured.
- Test users exist for admin, manager, cashier, fiscal, finance, privacy, and unauthorized/limited roles.
- Test tenant A and tenant B have separate products, sales, inventory, finance, fiscal, audit, and privacy data.

## Automated validation

Run these where appropriate:

```bash
cd backend
gofmt -l .
go test ./...
go vet ./...
```

```bash
cd web
npm ci
npm run lint
npm run build
```

## Manual functional flows

| Flow | Steps | Expected result |
|---|---|---|
| Login | Sign in with a valid active user. | Access token returned; refresh cookie set; audit event `auth.login` success. |
| Failed login | Try wrong password for an existing-format email. | Generic auth error; no user enumeration; audit event failure; rate limits apply. |
| Refresh session | Reload app or call refresh with cookie. | New access token; refresh cookie rotated; audit event `auth.refresh` includes tenant/user when identifiable. |
| Expired refresh | Force protected request to 401 and make refresh fail. | Access token is cleared and UI redirects immediately to `/login`. |
| Logout | Logout with a valid session. | Refresh token revoked; cookie cleared; frontend auth state cleared; audit event `auth.logout`. |
| Logout network failure | Abort the logout request before the backend responds. | UI remains authenticated and reports logout incomplete; after network recovery a successful logout clears the HttpOnly cookie and local auth state. |
| Logout revocation failure | Force the refresh-token store to fail revocation. | Backend returns `503`, does not expire the cookie and does not audit success; retry succeeds only after revocation is available. |
| Invalid refresh cookie | Attempt refresh with an invalid/expired refresh token. | Backend returns `401` and expires the invalid browser cookie. |
| Cash lifecycle | Open a cash session, attempt a second open, close it, close again, then reopen. | Second open and second close return conflict; reopen after valid close succeeds. |
| Cash reconciliation | Open with known amount, create a cash-method sale, close with a declared difference. | Response/database contain expected cash, declared amount and difference; `cash.open`/`cash.close` audit events exist with reconciliation metadata. |
| Sale creation | Create a sale with valid stock and payment. | Backend calculates totals; inventory decreases; finance/audit entries exist. |
| Sale double-submit | Trigger `Finalizar` twice in the same interaction window. | Exactly one sale POST and one idempotency key are emitted; sale count increases once. |
| Sale write-ahead failure | Make queue `localStorage.setItem` fail before finalization. | No sale POST leaves the browser; operator sees that no sale was sent. |
| Missing idempotency key | Call `POST /sales` without `Idempotency-Key`. | Request is rejected with `422`; no sale/stock/ledger mutation occurs. |
| Price tampering | Attempt sale with client-supplied low `unit_price`. | Backend ignores client price; total uses product/promotional price. |
| Insufficient stock | Attempt sale above available stock. | Standardized validation/conflict error; no stock mutation. |
| Sale cancellation | Cancel a finalized sale. | Stock restored in transaction; cancellation audit entry exists; no duplicate restoration. |
| Inventory update | Adjust stock with authorized role. | Movement and balance updated for authenticated tenant only. |
| Fiscal generation | Generate fiscal XML for a sale. | Fiscal record/XML reference created; audit event without full XML payload. |
| Fiscal access/download | Download fiscal XML as authorized fiscal role. | XML returned; access audited; unauthorized role denied. |
| Finance listing | Open dashboard/list ledger entries. | Tenant-scoped results only; pagination remains usable. |
| Audit log listing | Query audit logs as admin/audit role. | Tenant-scoped sanitized metadata returned; unauthorized role denied. |
| Privacy request create | Create a data subject request. | Request stored under authenticated tenant; audit event exists. |
| Privacy status transition | Move `open` to `in_progress`, then to `completed`. | Valid transitions succeed; terminal statuses cannot be changed again. |
| Privacy export | Export supported subject data. | Export includes current supported personal data and tenant-scoped related records. |
| Privacy anonymize/block | Run supported anonymize/block flow. | Direct personal fields removed or account inactivated; fiscal/accounting records retained. |
| Consent create/revoke | Record consent and revoke it. | Consent evidence remains; withdrawal timestamp set; audit events exist. |
| Offline enqueue | Disable network and create pending sale. | Queue stores only minimal sale payload, idempotency key, and timestamp. |
| Offline flush | Restore network and flush queue. | Sale submitted once; idempotent replay does not duplicate sale. |
| Lost response after commit | Let the backend process a sale, then fail the browser response before it is delivered. | Fallback queue preserves the original idempotency key; replay returns the prior result and sale count increases only once. |
| Offline permanent conflict | Put a permanently invalid/rejected item before a valid queued sale. | Invalid item is preserved as attention; later valid item still synchronizes. |
| Offline expiration | Leave a queued item older than 24 hours. | Item is not retried automatically and remains stored as attention; it is not silently deleted. |
| Browser tenant isolation | Switch tenant/user identity in the same browser with cached PDV state. | Cash session, product cache, and offline queue from the prior identity are inaccessible. |
| Legacy offline queue upgrade | Seed `sistemaemgo:offlineQueue:v1` and open PDV after login. | Legacy items are visible but never auto-sent; explicit import moves them to attention for manual review. |
| Legacy sale rebind after prior commit | Rebind an attention/legacy sale to the current cash after the original key was already committed. | Original idempotency key is preserved; changed payload receives conflict/attention and no second sale is created. |
| Offline reconciliation | Put an item in attention. | Operator can inspect the error, retry manually or discard with confirmation. |
| Offline logout | Logout with pending queue. | User is warned/confirmed before clearing pending items; no silent loss. |
| RBAC allow | Perform action with required permission. | Request succeeds. |
| RBAC deny | Perform same action with limited role. | Standardized `authorization_error`. |
| Tenant isolation | Authenticate real tenant A and tenant B users and cross sale, fiscal XML, finance, privacy and audit identifiers. | Cross-tenant direct lookups are not found and tenant B lists contain none of tenant A's IDs. |
| Tenant membership revocation | Remove the authenticated user from the token's tenant, then use access and refresh tokens again. | Protected access and refresh are rejected immediately even while the user account remains active. |
| Production TLS policy | Start staging/prod config with PostgreSQL below `sslmode=verify-full` or Redis without `rediss://`. | Startup config validation fails; production-like E2E succeeds only with certificate-verified PostgreSQL and Redis TLS. |
| Migration upgrade/rollback | Upgrade a populated v12 DB through v13/v14, roll back/reapply, inject duplicate open cash sessions for v13 and reconcile. | v13 rejects duplicate-open data explicitly; after reconciliation v13 applies; v14 cash-reconciliation columns survive validated down/up. |
| Redis limiter atomicity | Exercise the real Redis limiter repeatedly and inspect PTTL. | Requests are counted correctly and the rate-limit key always retains a positive TTL. |
| Redis limiter outage | Exercise a production-configured limiter with Redis unavailable. | Sensitive request returns `503 service_unavailable`; it does not silently fall back to a node-local allowance. |

## Restore drill smoke subset

After restoring a backup into isolated staging:

- Confirm schema version and app startup.
- Run login, refresh, sale creation/cancellation, inventory, fiscal listing, finance dashboard, audit listing, privacy request listing/export, and tenant isolation checks.
- Confirm Redis session loss behavior is acceptable if Redis was not restored.
- Record restore duration and any corrective actions.

## Evidence to capture

- CI run URL or command output.
- Migration command and result.
- Backup/restore artifact names.
- Request IDs for critical flows.
- Audit event IDs for auth, sale, fiscal, and privacy flows.
- Any failed checks and remediation tickets.

## Round 6 automated regression cases
- Sale vs cash-close concurrency: close waits on the cash-session lock and includes the committed in-flight sale.
- Cancel after cash close: returns `409` and the stored reconciliation remains immutable.
- Supply/withdrawal: cash movement, finance ledger and audit evidence are committed together; over-withdrawal is rejected.
- Mixed payment close: expected/declared/difference maps are checked for cash and non-cash methods.
- Fiscalized sale: normal cancellation after XML generation returns `409`.
- Redis legacy limiter key without TTL: next limiter call repairs a positive TTL.
- Migration path validates `v12 -> v16`, including rollback/reapply of `0015` and `0016`.
- UI tests wait for async cash-open/cash-close completion before continuing, preventing cross-test state leakage.
