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
| Logout | Logout with a valid session. | Refresh token revoked; cookie cleared; frontend auth state cleared; audit event `auth.logout`. |
| Sale creation | Create a sale with valid stock and payment. | Backend calculates totals; inventory decreases; finance/audit entries exist. |
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
| Offline logout | Logout with pending queue. | User is warned/confirmed before clearing pending items; no silent loss. |
| RBAC allow | Perform action with required permission. | Request succeeds. |
| RBAC deny | Perform same action with limited role. | Standardized `authorization_error`. |
| Tenant isolation | Tenant A user requests tenant B product/sale/privacy/audit IDs. | Access denied or not found; no cross-tenant data returned. |

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
