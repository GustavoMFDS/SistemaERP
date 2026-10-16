BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE code='sale:return:override');
DELETE FROM permissions WHERE code='sale:return:override';
ALTER TABLE sale_returns DROP COLUMN IF EXISTS return_basis;
ALTER TABLE sale_items DROP COLUMN IF EXISTS return_days_snapshot, DROP COLUMN IF EXISTS return_policy_snapshot;
ALTER TABLE products DROP COLUMN IF EXISTS return_days, DROP COLUMN IF EXISTS return_policy;
COMMIT;
