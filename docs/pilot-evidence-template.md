# Pilot evidence template

Copy this file for each pilot execution. Do not overwrite old evidence.

## Identification

- Pilot date:
- Store / tenant:
- PILOT_TENANT_ID:
- Register:
- Release SHA:
- Migration version:
- Environment:
- Operator(s):
- Manager on duty:
- Technical owner:
- Start time:
- End time:

## CI evidence

- Workflow run URL / ID:
- Head SHA matches release SHA: YES / NO
- Backend:
- Frontend:
- Integration:
- Security:
- E2E:
- E2E production-like:

## Preflight

- `scripts/pilot-readiness.sh` result:
- PILOT_TENANT_ID used:
- Exact command/environment reference:
- Command timestamp:
- Failures:
- Warnings:
- Runtime DB role elevated privileges: NONE / list
- Runtime DB role database CREATE: NO / YES
- Runtime DB role public-schema CREATE: NO / YES
- Reviewer:

## Backup and restore

- Backup drill evidence file:
- Backup artifact:
- Backup checksum:
- Backup timestamp:
- Restore target:
- Restore start/end:
- Restored migration version / dirty:
- Critical tables found:
- Source representative counts:
- Restore representative counts:
- Strict row-count mode used: YES / NO
- Reviewer:

## Monitoring

- Dashboard/reference:
- Metrics-unavailable alert tested:
- Readiness alert tested:
- HTTP 5xx alert tested:
- p95 latency alert tested:
- Alert delivery destination:
- On-call / stop authority:

## Resilience and load

- Loadcheck target/scenario:
- Command/options reference:
- Duration:
- Concurrency:
- Requests / RPS:
- Error rate / threshold:
- p50:
- p95 / threshold:
- p99:
- Load result: PASS / FAIL
- Redis outage detected by readiness: YES / NO
- Redis recovery detected: YES / NO
- PostgreSQL outage detected by readiness: YES / NO
- PostgreSQL recovery detected: YES / NO
- Failure-drill evidence file:
- Reviewer:

## Database credential separation

- Runtime application DB role:
- Migrator/owner role separate: YES / NO
- Backup role separate: YES / NO
- Runtime role SUPERUSER/CREATEDB/CREATEROLE/REPLICATION/BYPASSRLS: NONE / list
- Runtime role database CREATE: NO / YES
- Runtime role public-schema CREATE: NO / YES
- Reviewer:

## Data load reconciliation

| Measure | Source | Imported/system | Difference | Reviewer |
|---|---:|---:|---:|---|
| Products | | | | |
| Barcoded products | | | | |
| Opening inventory sample | | | | |
| Suppliers | | | | |

## Scenario evidence

| Scenario | IDs / timestamps / request IDs | Expected | Actual | PASS/FAIL | Notes |
|---|---|---|---|---|
| Cash open | | | | | |
| Duplicate cash open conflict | | | | | |
| Barcode sale | | | | | |
| Search sale | | | | | |
| Authorized discount | | | | | |
| Cashier discount denied | | | | | |
| Suspended/resumed cart | | | | | |
| Offline sale + reconnect | | | | | |
| Purchase partial receipt | | | | | |
| Purchase final receipt | | | | | |
| Return with restock | | | | | |
| Return without restock | | | | | |
| Exchange + new sale | | | | | |
| Payment reconciliation | | | | | |
| Reconciliation divergence | | | | | |
| Refund settlement | | | | | |
| Cash close | | | | | |

## Reconciliation sample

- Cash opening:
- Cash sales:
- Supplies:
- Withdrawals:
- Cash refunds:
- Expected physical cash:
- Declared physical cash:
- Difference:

| Method | Sales | Refunds tied to session | Expected net | Declared/settled | Difference |
|---|---:|---:|---:|---:|---:|
| PIX | | | | | |
| Debit | | | | | |
| Credit | | | | | |
| Transfer | | | | | |
| Voucher | | | | | |

## Incidents / defects

| Severity | Description | Evidence | Workaround | Owner | Resolved? |
|---|---|---|---|---|---|
| | | | | | |

## Stop conditions triggered

- None / list:

## Approvals

These signatures mean the pilot evidence was reviewed. They do not replace legal or tax advice.

- Store owner / operations:
- Technical owner:
- Accountant / fiscal reviewer:
- Privacy/DPO reviewer, if applicable:

## Decision

- [ ] NO-GO — return to existing process and resolve findings.
- [ ] CONTINUE PILOT — more evidence required.
- [ ] GO FOR NON-FISCAL EXPANSION — operational evidence accepted for the tested scope.

Fiscal production approval: **NOT INCLUDED**.
