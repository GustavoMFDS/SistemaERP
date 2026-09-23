-- 0016_idempotency_retention_index.down.sql
BEGIN;

DROP INDEX IF EXISTS idempotency_keys_created_at_idx;

COMMIT;
