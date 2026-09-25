# Records of processing activities template

Use this template for each processing activity and validate the legal basis with counsel.

| Field | Value |
|---|---|
| Activity |  |
| Controller / tenant |  |
| Data subjects |  |
| Personal data categories |  |
| Purpose |  |
| Suggested legal basis |  |
| Systems/tables |  |
| Access roles |  |
| Retention |  |
| Processors / sharing |  |
| Security controls |  |
| Data subject rights handling |  |
| International transfer |  |
| Risk notes |  |

Initial activity map:

- User creation: `users`, `user_roles`, `user_tenants`, audit action `user.created`.
- Login/logout: Redis refresh token store, `users.last_login_at`, audit actions `auth.login`, `auth.logout`.
- Sale: `sales`, `sale_items`, `payments`, `ledger_entries`, audit action `sale.created`.
- Customer creation/update: `customers`, audit actions `customer.created`, `customer.updated`.
- Data subject request: `data_subject_requests`, audit actions `privacy.request.create`, `privacy.request.update`; request writes and audits share one transaction.
- Export: `/api/v1/privacy/requests/{id}/export`, audit action `privacy.subject.export`; an `in_progress` export DSR is required and data is released only after audit persistence succeeds.
- Blocking/anonymization: data subject workflow, audit actions `privacy.subject.block`, `privacy.subject.anonymize`; subject mutation, DSR completion, and audit commit atomically.
- Consent: `consent_records`, audit actions `privacy.consent.create`, `privacy.consent.revoke`; consent mutation and audit commit atomically.
- Audit review: `/api/v1/audit/logs`, audit metadata is sanitized before persistence and output.
