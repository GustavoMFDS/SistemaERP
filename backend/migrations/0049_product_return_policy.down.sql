BEGIN;
ALTER TABLE sale_items DROP COLUMN IF EXISTS return_days_snapshot, DROP COLUMN IF EXISTS return_policy_snapshot;
ALTER TABLE products DROP COLUMN IF EXISTS return_days, DROP COLUMN IF EXISTS return_policy;
COMMIT;
