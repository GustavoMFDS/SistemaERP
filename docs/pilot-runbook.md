# Pilot runbook — one-store controlled validation

This runbook prepares a real-store pilot. It is not production approval and must not be marked complete until evidence comes from the actual pilot environment.

## Scope

Use one store/tenant, one register, a small trained operator group, and a production-like environment.

Keep fiscal issuance disabled. The pilot validates only the non-fiscal cycle:

1. product and opening-stock load;
2. cash open;
3. barcode/search sale;
4. payment recording and reconciliation;
5. suspended/offline cart behavior;
6. purchase receiving;
7. return/exchange;
8. refund settlement;
9. cash close and method reconciliation;
10. audit and recovery evidence.

## Entry criteria

All items are mandatory before operator training.

- [ ] Exact release SHA recorded.
- [ ] CI executed on that exact SHA and all required gates are green.
- [ ] APP_ENV is staging/prod-like.
- [ ] PostgreSQL uses verified TLS.
- [ ] Redis uses authenticated TLS.
- [ ] Metrics are authenticated.
- [ ] Demo seed is disabled.
- [ ] Fiscal provider is disabled until a SEFAZ-ready integration exists.
- [ ] Demo users are absent.
- [ ] Real tenant/company exists with a current DFe-compatible CNPJ (14 normalized characters: first 12 alphanumeric, final 2 numeric).
- [ ] At least one active cash register exists and no cash session is already open at pilot start.
- [ ] Active product pricing is consistent (`promo_price` never exceeds `price_cash`).
- [ ] At least one admin/manager and one cashier have individual accounts.
- [ ] Tenant-scoped roles reviewed.
- [ ] Runtime PostgreSQL role passes least-privilege checks (no superuser/role/db/DDL creation privileges).
- [ ] Migration/owner, application and backup credentials are separated.
- [ ] Schema version is clean and at least 29.
- [ ] Pre-pilot backup created with `scripts/postgres-backup-drill.sh`.
- [ ] That backup restored into an isolated database and the generated evidence reports PASS.
- [ ] Monitoring/alerts active for API availability, readiness, 5xx and latency.
- [ ] Alert delivery tested end-to-end to the person with stop authority.
- [ ] Controlled load smoke completed with recorded p95/error-rate thresholds.
- [ ] Redis/PostgreSQL failure/recovery drill completed in an approved non-production target.
- [ ] Owner/accountant understands the fiscal boundary of this pilot.

Run the read-only preflight against the exact tenant selected for the pilot:

```bash
APP_ENV=staging \
API_BASE_URL=https://pilot.example.com \
DATABASE_URL='postgres://.../?sslmode=verify-full&sslrootcert=/path/to/ca.crt' \
PILOT_TENANT_ID='<uuid-da-loja-piloto>' \
FISCAL_PROVIDER=disabled \
ALLOW_DEMO_SEED=0 \
./scripts/pilot-readiness.sh
```

`PILOT_TENANT_ID` is mandatory. User, membership and permission checks are evaluated only for that tenant, so another configured store cannot make the target store appear ready.

A successful preflight does not replace the manual checks above.

## Data preparation

- Export the current trusted product source.
- Normalize SKU, description, unit, barcode, cost, price and active status.
- Resolve duplicate barcodes before import.
- Load opening inventory from a physical count or other trusted source.
- Have a second person review a sample of opening balances.
- Register suppliers used during the pilot.
- Do not create fake fiscal documents to mimic real issuance.

Record before opening the first cash session:

- products:
- active products:
- products with barcode:
- suppliers:
- inventory sample/count:
- users:
- tenant-scoped roles:

## Operator setup

Use individual accounts.

Recommended separation:

- cashier/operator: normal sale and cash lifecycle;
- manager: discounts, returns/exchanges and reconciliation;
- admin: configuration and emergency access.

Before opening:

- scanner works as keyboard input;
- browser clock/timezone is correct;
- non-fiscal receipt printer works if used;
- network-loss test can be performed safely;
- operator understands suspended cart vs offline finalized intent;
- manager understands attention-state reconciliation.

## Controlled parallel operation

For the first pilot window, keep the store's current trusted process running in parallel.

Recommended first window:

- one register;
- one operator at a time;
- representative cash and digital payments;
- limited sample of real transactions agreed with owner/accountant;
- manager available to resolve discrepancies.

For sampled sales compare:

- item/quantity;
- price/discount;
- payment method;
- stock movement;
- cash-session association;
- finance ledger;
- reconciliation state;
- audit event.

## Mandatory scenarios

### Cash

- [ ] Open with known amount.
- [ ] Duplicate open is rejected.
- [ ] Supply/withdrawal if used.
- [ ] Close with independently counted cash.
- [ ] Compare expected, declared and difference.

### Sale

- [ ] Barcode sale.
- [ ] Quick-search sale.
- [ ] Quantity edit.
- [ ] Authorized discount.
- [ ] Cashier cannot force discount.
- [ ] Suspend and resume cart.
- [ ] Non-fiscal receipt if used.

### Offline / failure

Perform only in a controlled window.

- [ ] Finalize one sale with network unavailable.
- [ ] Tentativa de fechar caixa com venda offline pendente é bloqueada até sincronização/reconciliação.
- [ ] Exact intent remains pending.
- [ ] Restore network and confirm a single server sale.
- [ ] Rejected item remains visible for attention.
- [ ] No discard/rebind without manager review.

### Purchasing

- [ ] Create purchase.
- [ ] Receive partially.
- [ ] Stock/cost changes only on receipt.
- [ ] Receive remainder.
- [ ] Compare sample physical stock.

### Return/exchange

- [ ] Partial return with restock.
- [ ] Damaged return without restock.
- [ ] Exchange followed by a new normal sale.
- [ ] Full cancellation blocked after partial return.

### Payments/refunds

- [ ] Reconcile one PIX/card payment.
- [ ] Record provider fee.
- [ ] Exercise and resolve one divergence.
- [ ] Settle a return refund.
- [ ] Cash refund produces physical withdrawal when used.
- [ ] Method-level close uses net digital values.

## Backup/restore evidence

Use `docs/backup-restore-runbook.md` and run:

```bash
SOURCE_DATABASE_URL='postgres://.../sistemaemgo?sslmode=verify-full' \
RESTORE_DATABASE_URL='postgres://.../sistemaemgo_restore_validation?sslmode=verify-full' \
ALLOW_RESTORE_RESET=1 \
bash scripts/postgres-backup-drill.sh
```

Do not mark backup/restore as tested unless the generated evidence records PASS and the operator retains:

- artifact location;
- SHA-256 checksum;
- start/end timestamps;
- isolated restore target;
- restored migration version;
- critical-table validation;
- representative row counts.

For a quiesced maintenance window, use `STRICT_ROW_COUNTS=1`.

Restore only into an explicitly disposable target.

## Monitoring

Use `docs/monitoring-runbook.md` as the baseline. The repository includes Prometheus + Blackbox configuration and baseline alerts for:

- API metrics availability;
- `/health/ready`;
- HTTP 5xx rate;
- p95 HTTP latency.

The production monitoring platform must authenticate to `/metrics`; the local Compose overlay is only a validation stack.

Also watch PostgreSQL errors/storage, Redis errors, authentication/rate-limit failures, operator-reported offline attention items, cash differences and payment reconciliation divergences.

Assign one named person with authority to stop the pilot and prove alert delivery to that person before pilot entry.

## Resilience and load evidence

Use `docs/resilience-load-runbook.md`.

Record a bounded read-load smoke with `backend/cmd/loadcheck` and preserve:

- URL/scenario;
- concurrency and duration;
- request count/RPS;
- error rate;
- p50/p95/p99;
- configured acceptance thresholds.

Then execute `scripts/dependency-failure-drill.sh` only in an approved non-production target and prove:

- Redis outage makes readiness fail and recovery returns it to 200;
- PostgreSQL outage makes readiness fail and recovery returns it to 200;
- monitoring raises the expected readiness/availability alert.

## Database credentials

Use `docs/database-least-privilege.md`.

The runtime `DATABASE_URL` must identify the application role, not the migrator/owner. `scripts/pilot-readiness.sh` rejects elevated cluster privileges and database/schema CREATE capability.

## Stop conditions

Stop new pilot transactions and fall back to the existing process if any occurs:

- duplicate or irreconcilably ambiguous sale;
- unexplained stock corruption;
- untraceable cash difference;
- cross-tenant data visibility;
- authentication/permission bypass;
- repeated database/Redis readiness failure;
- backup cannot be restored;
- operator cannot safely resolve offline attention items;
- records become less reliable than the existing process.

Preserve logs, request IDs, sale IDs, cash-session IDs and timestamps.

## Exit decision

Pilot evidence is complete only when:

- all mandatory scenarios have evidence;
- no unresolved high-severity issue remains;
- sampled stock/cash/payment results reconcile;
- backup/restore evidence is complete;
- load/failure evidence is complete;
- runtime database least-privilege preflight passes;
- owner/operations accepts the workflow;
- accountant reviews the non-fiscal boundary;
- privacy/DPO review is recorded when applicable;
- monitoring and support ownership are assigned and alert delivery is proven.

Expansion to additional stores should repeat the same checklist for each tenant/store.

Fiscal production remains a separate gate.
