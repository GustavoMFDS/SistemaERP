# Privacy by design checklist

Use this checklist before adding or changing features.

- Define the purpose for each personal data field before collecting it.
- Confirm the suggested legal basis with legal/privacy counsel.
- Avoid returning personal fields in API responses unless the screen needs them.
- Require authentication and authorization by default for new routes.
- Scope every query by `tenant_id`; never trust tenant identifiers from the client when the authenticated context already has one.
- Add audit logging for sensitive reads/writes and data exports.
- Do not log secrets, tokens, passwords, raw documents, or excessive personal data.
- Prefer anonymization over deletion when fiscal/accounting retention applies.
- Add retention behavior or document why retention is legal-obligation based.
- Add tests for cross-tenant isolation and authorization.
- If a feature collects consent, record consent purpose, text/version, source, timestamp, and withdrawal through the privacy API.
- If a feature stores personal data offline, document TTL, minimum required fields, and limitations of browser storage.
- Update `docs/privacy/data-inventory.md` and `docs/privacy/ropa.md`.
