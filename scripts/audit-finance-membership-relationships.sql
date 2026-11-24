-- Run read-only on a verified database copy before v54. Counts only.
SELECT 'ledger_entries.sale_id' AS relation, count(*) AS mismatches
FROM ledger_entries l LEFT JOIN sales s ON s.id=l.sale_id AND s.tenant_id=l.tenant_id
WHERE l.sale_id IS NOT NULL AND s.id IS NULL
UNION ALL
SELECT 'ledger_entries.cash_session_id', count(*)
FROM ledger_entries l LEFT JOIN cash_sessions c ON c.id=l.cash_session_id AND c.tenant_id=l.tenant_id
WHERE l.cash_session_id IS NOT NULL AND c.id IS NULL
UNION ALL
SELECT 'idempotency_keys.sale_id', count(*)
FROM idempotency_keys k LEFT JOIN sales s ON s.id=k.sale_id AND s.tenant_id=k.tenant_id
WHERE s.id IS NULL
UNION ALL
SELECT 'user_tenant_roles.membership', count(*)
FROM user_tenant_roles r LEFT JOIN user_tenants m ON m.user_id=r.user_id AND m.tenant_id=r.tenant_id
WHERE m.user_id IS NULL;
