# PostgreSQL backup/restore drill

This runbook turns the production-readiness backup requirement into a repeatable, auditable drill.

The repository script is:

```bash
bash scripts/postgres-backup-drill.sh
```

It creates a PostgreSQL custom-format dump, records a SHA-256 checksum, restores it into a **disposable** database, validates the restored schema, checks 44 baseline pilot tables and, when installed, the photo and variant tables, compares representative row counts, and writes an evidence file.

## Safety model

The script is intentionally destructive **only for the restore database**. It drops and recreates the `public` schema in `RESTORE_DATABASE_URL`.

Protections:

- source and restore URLs must be different;
- source and restore database names must resolve to different databases;
- the restore database name must contain `restore`, `backup`, `drill`, `validation`, or `test`;
- `ALLOW_RESTORE_RESET=1` is mandatory;
- the source database must have a clean migration state;
- the source schema must be at least version 32;
- credentials are not written to the evidence file.

Never point `RESTORE_DATABASE_URL` at staging or production data that must be preserved.

## Prerequisites

Install PostgreSQL client tools compatible with the server major version:

- `pg_dump`
- `pg_restore`
- `psql`
- `sha256sum`

Create a separate disposable database on the target PostgreSQL server, for example:

```text
sistemaemgo_restore_validation
```

The restore user needs permission to drop/create objects inside that database.

## Production-like drill

Example:

```bash
export SOURCE_DATABASE_URL='postgres://.../sistemaemgo?sslmode=verify-full'
export RESTORE_DATABASE_URL='postgres://.../sistemaemgo_restore_validation?sslmode=verify-full'
export ALLOW_RESTORE_RESET=1
export BACKUP_DIR='./artifacts/backups'

bash scripts/postgres-backup-drill.sh
```

Successful execution produces:

- `sistemaemgo-<UTC timestamp>.dump`
- `sistemaemgo-<UTC timestamp>.dump.sha256`
- `sistemaemgo-<UTC timestamp>.dump.evidence.txt`

The restored database is intentionally left intact for manual inspection after the drill.

## Strict row-count mode

For a maintenance window where the source is quiesced, enable exact comparison of representative table counts:

```bash
export STRICT_ROW_COUNTS=1
bash scripts/postgres-backup-drill.sh
```

The strict comparison covers:

- companies;
- users;
- products;
- sales;
- audit logs.

Do not enable strict mode against a database receiving live writes, because legitimate writes after the snapshot can change source counts while the restore is being verified.

## Evidence to retain

For each drill retain, in access-controlled storage:

1. the evidence file;
2. the dump checksum;
3. the backup object identifier/path in the backup system;
4. execution date and operator;
5. observed restore duration;
6. any corrective actions.

Do not commit production dumps, checksums tied to private storage paths, credentials, or production evidence containing sensitive operational details to Git.

## Operational acceptance

A backup drill is considered successful when:

- `pg_dump` completes;
- checksum verification passes;
- `pg_restore` completes with `--exit-on-error`;
- restored `schema_migrations` is clean and matches the source version;
- all 44 baseline pilot tables exist, plus `product_images` from schema 36 and `product_variations` from schema 38 when applicable;
- representative row counts are captured;
- the evidence file reports `Result: PASS`.

A repository-only test does **not** prove production backup readiness. Before a real-store pilot, execute this drill against the actual backup path and production-like PostgreSQL infrastructure and retain the evidence outside the repository.

## Recommended cadence

Before pilot entry, execute at least one full restore drill. After launch, define the business RPO/RTO and schedule backup plus restore verification according to those requirements. A backup that has never been restored should not be treated as verified.


## App/browser decision for the family stores

The product should remain a responsive web application with an installable
**PWA**, rather than an Android/Windows native rewrite at this stage. The
same HTTPS deployment works on computers, phones and tablets and provides
a home-screen/desktop icon in browsers that support installation.

The current PWA service worker caches **app shell assets only**, not APIs,
invoices, customer details or authentication. Offline sales use the separate
tenant/user-scoped write-ahead queue: the operator must keep the device/browser
storage intact until every pending sale has been reconciled. Creating a PWA
icon does **not** make fiscal transmission work without SEFAZ connectivity.

Validate actual pilot equipment *before launch*:
- USB barcode scanners in keyboard-wedge mode: enter code + Enter, including
  back-to-back scans and the device's configured keyboard layout.
- Normal A4/office receipt printers: browser print dialogs, margins and
  cross-browser tests on the exact host OS.
- Thermal ESC/POS printer, cash drawer and serial/Bluetooth equipment:
  do not assume silent printing or device control works across Safari,
  Firefox and Chromium. Use a vendor-supported driver or a small
  audited **local companion service** only if a pilot device needs one.
- Intermittent network: queued sales, retry with original idempotency key,
  stock revalidation when reconnecting and refusal to close a cash session
  with pending sales.
- Shared or lost devices: independent company/user sessions, offline
  queue privacy, safe logout and device/browser storage retention.

Only consider a native client once an actual deployment requirement (specific
printer/TEF hardware, kiosk restrictions, sustained disconnected operation,
or OS integration) has been demonstrated and cannot be met by the PWA and a
small bridge. No new store app distribution is required for the current pilot.
