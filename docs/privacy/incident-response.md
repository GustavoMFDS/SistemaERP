# Incident response

This is a technical response flow for security and privacy incidents.

1. Identification: collect alert source, request IDs, tenant IDs, affected resources, timestamps, IPs, user agents, and suspicious actions.
2. Containment: revoke sessions, rotate secrets, disable compromised accounts, block abusive origins/IPs, and preserve evidence.
3. Investigation: correlate structured logs, `audit_logs`, database changes, Redis session state, and deployment history.
4. Impact assessment: identify affected data subjects, data categories, tenants, systems, and whether fiscal/legal records are involved.
5. Internal communication: notify engineering, security, privacy/DPO contact, leadership, and legal counsel.
6. Evidence collection: export immutable copies of relevant logs, audit rows, hashes, configs, and timelines.
7. Post-incident review: document root cause, corrective actions, owners, deadlines, and monitoring improvements.

If personal data may be affected, open a data subject/privacy incident record through the privacy workflow, preserve related `request_id` values, and involve the configured privacy/DPO contact before external communication decisions.

Traceability requirements:

- Every sensitive API action should include `request_id` in logs and error responses.
- Audit logs should include `tenant_id`, `actor_user_id`, action, resource type/id, timestamp, IP/user agent when appropriate, and `request_id`.
- Never paste tokens, passwords, or secret values into incident tickets.
- Collect backup IDs, restore test results, deployment SHAs, migration versions, and rollback decisions for the incident timeline.
