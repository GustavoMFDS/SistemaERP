-- 0015_cash_movements_and_reconciliation.down.sql
BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE code='cash:move');
DELETE FROM permissions WHERE code='cash:move';

DROP TABLE IF EXISTS cash_session_reconciliations;
DROP TABLE IF EXISTS cash_movements;

COMMIT;
