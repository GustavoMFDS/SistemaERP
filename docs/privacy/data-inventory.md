# Data inventory

This document supports LGPD governance and must be validated by legal counsel. It is not a legal compliance guarantee.

| Data | Purpose | Suggested legal basis to validate | Retention | Access | Storage | Sharing |
|---|---|---|---|---|---|---|
| User name, email, password hash, roles | Authentication, authorization, accountability | Contract execution / legitimate interest | While account is active plus audit retention | Tenant admins, system operators | PostgreSQL `users`, RBAC tables | Not shared by default |
| Login timestamps and audit metadata | Security monitoring and traceability | Legitimate interest / legal obligation | 6 to 24 months, configurable | Security/admin roles | PostgreSQL `audit_logs`, app logs | Incident responders if needed |
| Customer name, document, email, phone | Sales/customer service and fiscal workflows | Contract execution / legal obligation / consent where applicable | Active relationship plus fiscal/legal retention | Sales, finance, fiscal roles | PostgreSQL `customers`, sales/fiscal records | Fiscal providers when configured |
| Sale and payment metadata | POS operation, financial records, fiscal evidence | Contract execution / legal obligation | Fiscal/accounting statutory period | Sales, finance, fiscal roles | PostgreSQL `sales`, `payments`, `ledger_entries` | Accountants/fiscal services when configured |
| IP, user agent, request id | Fraud prevention, incident investigation | Legitimate interest | Short operational retention | Security/admin roles | `audit_logs`, structured logs | Incident responders if needed |
| Data subject request records | LGPD rights workflow evidence | Legal obligation | 5 years or legal-counsel defined | Privacy/admin roles | `data_subject_requests` | Legal/privacy counsel |
| Consent records, when used | Evidence of consent and withdrawal | Consent | Until withdrawn plus evidence retention | Privacy/admin roles | `consent_records` | Not shared by default |

Minimization notes:

- Passwords are never stored, only bcrypt hashes.
- Refresh token identifiers are stored in Redis; raw tokens must not be logged.
- Fiscal/accounting data should not be deleted automatically when legal retention applies; prefer restriction/anonymization of non-fiscal personal fields.
- Privacy exports currently support tenant-scoped `customer` and `user` subjects. Extend the export map deliberately when new personal-data tables are added.
- Current exports include direct subject fields plus supported related sales references, data subject requests, consent records, and user audit event references. They do not export full fiscal XML payloads.
- TODO for future domain expansion: if customer CPF/address fields or additional fiscal recipient fields are introduced, update privacy export/anonymization logic and this inventory in the same change.
