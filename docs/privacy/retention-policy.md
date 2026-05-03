# Retention and disposal policy

This technical policy supports LGPD processes and must be aligned with tax/accounting obligations by legal counsel.

Configuration:

- `PRIVACY_CONTACT_EMAIL`: public privacy/DPO channel.
- `APP_PUBLIC_URL`: public application URL used in privacy communications.
- Future retention job variables should define windows for logs, inactive users, customers without fiscal records, and data subject request records.

Default technical approach:

- Fiscal, sale, ledger, invoice, and accounting records are retained for the legally required period. Do not hard-delete these records through automated privacy jobs.
- Customer contact fields may be anonymized when there is no legal obligation or active relationship requiring retention.
- Audit logs are retained long enough to investigate incidents, typically 6 to 24 months depending on risk and legal guidance.
- Refresh token records expire automatically in Redis based on `REFRESH_TOKEN_TTL_MINUTES`.

Disposal workflow:

1. Identify records eligible for disposal by tenant and data subject.
2. Check legal holds, fiscal/accounting linkage, and open disputes.
3. Prefer anonymization over deletion where records must remain for integrity.
4. Execute inside a transaction and write an audit log with `tenant_id`, actor, action, resource, timestamp, IP/user agent, and `request_id`.
5. Record the data subject request outcome in `data_subject_requests`.

Implementation status:

- Migration `0006_privacy_audit_controls` prepares `data_subject_requests` and `consent_records`.
- The API exposes RBAC-protected privacy endpoints under `/api/v1/privacy` for request tracking, export, anonymization, blocking, consent recording, and consent revocation.
- Data subject request status flow is `open` -> `in_progress` -> `completed`, with `rejected` and `cancelled` terminal alternatives where applicable. Direct `open` -> `rejected` and `open` -> `cancelled` are allowed when appropriate.
- Terminal statuses (`completed`, `rejected`, `cancelled`) cannot be changed through the status endpoint, including same-status updates. Use a future admin-note mechanism if post-closure commentary is required.
- Customer anonymization clears direct contact/document fields. User anonymization/inactivation preserves fiscal/accounting references while removing direct account identifiers where supported.
- Exports include direct subject fields plus currently related sales/invoice identifiers for customers, consent records, privacy requests, and user audit event references where applicable.
- Fiscal, sale, ledger, and invoice records are not hard-deleted by privacy endpoints; they must be retained or restricted according to legal/accounting validation.
- Automated anonymization/delete jobs are still pending and should be implemented as explicit commands before production use.
