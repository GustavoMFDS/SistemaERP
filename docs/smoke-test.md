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
| Barcode scanner | In PDV, scan a known barcode twice using a keyboard-mode USB/Bluetooth scanner or type the code and press Enter twice. | The first scan adds the product; the second increments the same cart line to quantity 2. Unknown offline codes are rejected without inventing a product. |
| PDV keyboard shortcuts | Open PDV and press F2, F4 and F8 during a valid cart flow. | F2 focuses barcode, F4 focuses quick search and F8 triggers finalization only when Finalizar is enabled. |
| Suspended cart | Add quantity 2, apply authorized discount, suspend the cart and resume it. | Cart clears without creating a server sale; resumed cart restores items/payment/discount only for the same tenant+user. |
| Procurement RBAC | Login as cashier and access menu/API for suppliers and purchases. | Purchases menu is absent; direct supplier/purchase API calls return 403. |
| Cost/profit redaction | Compare product and sale detail as admin vs cashier. | Admin sees real cost/profit; cashier sees zeros while sale totals remain unchanged. |
| Reserved inventory movement types | Submit manual inventory adjustment using type purchase or return. | Request is rejected with 422; only adjustment/loss/damage are accepted manually. |
| Payment reconciliation history | Reconcile a digital payment divergently, replay the same idempotency key, then submit a second initial reconciliation with a new key. | Same key replays the original result; the second initial reconciliation returns 409 and the immutable initial history remains singular. |
| Reconciliation fee ledger | Reconcile a payment with a fee, then adjust that fee and replay the adjustment key. | Ledger contains `payment_fee` entries whose sum equals the current fee with negative sign; replay creates no duplicate fee entry. |
| Reconciliation adjustment | After a divergent reconciliation, correct received amount/fee with a justification, replay the adjustment key, then open reconciliation history. | A separate immutable adjustment row records old/new values, payment status becomes reconciled when corrected, one `payment.reconcile.adjust` audit event exists, same-key replay returns the original adjustment, and history shows the divergent initial record followed by the correction. |
| Purchase audit atomicity | Create, partially/finally receive and cancel an unreceived purchase. | Audit contains one create, one event per real receipt and one cancel; cancelled purchase cannot be received. |
| Discount RBAC | As admin/manager submit a discounted sale, then call the same flow as cashier using a crafted API request. | Authorized discount succeeds; cashier receives 403; no discounted sale is created for cashier. |
| Non-fiscal receipt | Finalize an online sale. | UI shows sale ID/total and an explicit non-fiscal print action. |
| Numeric overflow guard | Submit price/quantity combinations whose computed line or aggregate total exceeds the database numeric range. | Request is rejected as invalid; no sale/purchase/stock/ledger mutation occurs and no wrapped negative/zero amount is produced. |
| Sale creation | Create a sale with valid stock and payment. | Backend calculates totals; inventory decreases; finance/audit entries exist. |
| Sale double-submit | Trigger `Finalizar` twice in the same interaction window. | Exactly one sale POST and one idempotency key are emitted; sale count increases once. |
| Sale write-ahead failure | Make queue `localStorage.setItem` fail before finalization. | No sale POST leaves the browser; operator sees that no sale was sent. |
| Missing idempotency key | Call `POST /sales` without `Idempotency-Key`. | Request is rejected with `422`; no sale/stock/ledger mutation occurs. |
| Price tampering | Attempt sale with client-supplied low `unit_price`. | Backend ignores client price; total uses product/promotional price. |
| Insufficient stock | Attempt sale above available stock. | Standardized validation/conflict error; no stock mutation. |
| Partial return | Finalize a sale with qty 2, return qty 1 with `restock=true`, replay the same idempotency key, then attempt qty 2 again. | Stock is credited exactly once, replay returns the same return ID, over-return is rejected, and full sale cancellation is blocked afterward. |
| Damaged exchange item | Return the remaining item with `kind=exchange` and `restock=false`. | Refund due is calculated, but sellable stock does not increase; refund status remains pending. |
| Sale cancellation | Cancel a finalized sale. | Stock restored in transaction; cancellation audit entry exists; no duplicate restoration. |
| Inventory update | Adjust stock with authorized role using adjustment/loss/damage, then try to submit purchase/return through the manual endpoint. | Authorized manual movement updates the tenant balance; reserved purchase/return types return 422 and must come from their dedicated flows. |
| Fiscal generation | Generate fiscal XML for a sale. | Fiscal record/XML reference created; audit event without full XML payload. |
| Fiscal access/download | Download fiscal XML as authorized fiscal role. | XML returned; access audited; unauthorized role denied. |
| Payment reconciliation | Create a PIX/card sale, reconcile it with received amount, fee, provider and external reference, replay the same idempotency key, then retry the same payment with a different key. | Reconciliation history is written once; same-key replay returns the original result; a second reconciliation with another key returns 409; sale total is unchanged. |
| Digital sale cancellation | Create a PIX/card sale and try full cancellation. | Cancellation returns 409; use return + refund settlement instead. Cash-only cancellation remains available while original session is open. |
| Return refund settlement | Create a return with refund due, settle part in cash, settle the remainder by PIX, then replay the original cash settlement after the return is fully settled. | Cash withdrawal is recorded once; refund moves pending → partial → settled; the late same-key replay returns the original refund ID/status/remaining amount, over-refund returns conflict, and ledger contains no duplicate return_refund entry. |
| Net method close | In one open session, create a PIX R$20 sale, refund R$15 PIX tied to that session and R$5 cash. | Closing expects PIX R$5 and physical cash reduced by the R$5 withdrawal, with no double subtraction. |
| Net-negative digital close | Refund by PIX in a new session a R$10 sale from a prior closed session, with no new PIX sale in the refund session. | Closing accepts expected/declared PIX = -R$10.00 and records zero difference; physical cash remains non-negative. |
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
| Cashier cost redaction | As cashier read product list/detail/barcode, low-stock and sale detail. | Product cost, sale item cost and estimated profit are zero/redacted; operational price/quantity fields remain usable. |
| Cashier write controls | Login as cashier and visit Products/Inventory. | Product create/barcode-save and inventory-adjust controls are not rendered; direct write requests are still denied by backend RBAC. |
| Return module RBAC | As cashier call return list/detail directly. | Request returns 403 because return reads require `sale:return`. |
| Tenant isolation | Authenticate real tenant A and tenant B users and cross sale, fiscal XML, finance, privacy and audit identifiers. | Cross-tenant direct lookups are not found and tenant B lists contain none of tenant A's IDs. |
| Tenant membership revocation | Remove the authenticated user from the token's tenant, then use access and refresh tokens again. | Protected access and refresh are rejected immediately even while the user account remains active. |
| Production TLS policy | Start staging/prod config with PostgreSQL below `sslmode=verify-full` or Redis without `rediss://`. | Startup config validation fails; production-like E2E succeeds only with certificate-verified PostgreSQL and Redis TLS. |
| Procurement retry idempotency | Repeat supplier creation, purchase creation, and a 1.5-unit receipt with the exact same Idempotency-Key and body. | Replays return the original resource with replayed=true; only one supplier/order/receipt is created and stock is credited once. Reusing a key with a different body returns 409. |
| Purchase partial receiving | Create a purchase for 4 units without receiving it, then receive 1.5 and later 2.5. | Stock is unchanged at purchase creation, rises by 1.5 on the first receipt and reaches 4 on the second; purchase status moves ordered → partially_received → received; two purchase receipt movements exist. |
| Purchase active entities | Try creating a purchase with an inactive supplier or inactive product. | Request is rejected with 422; no purchase/payable/stock mutation occurs. |
| Purchase cancellation | Create an unreceived purchase with a due date, cancel it, then try to receive it. | Purchase becomes cancelled, open payable is cancelled, later receipt returns 409 and one transactional `purchase.cancel` audit event exists. |
| Procurement least privilege | Login as cashier and open navigation/direct supplier and purchase URLs. | Compras is hidden and supplier/purchase API reads return 403. |
| Purchase tenant isolation | Create suppliers/purchases in two tenants and try cross-tenant direct lookup. | Same supplier document may exist in different tenants; same-tenant duplicates are rejected; cross-tenant purchase lookup is not found. |
| Migration upgrade/rollback | Upgrade a populated v12 DB through v22, roll back/reapply, inject duplicate open cash sessions for v13 and reconcile. | v13 rejects duplicate-open data explicitly; v14 cash-reconciliation columns survive down/up; v17 barcode, v18 procurement, v19 returns, v20 reconciliation/refunds, v21 PDV permission and v22 reconciliation-adjustment history are present after reapply. |
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
