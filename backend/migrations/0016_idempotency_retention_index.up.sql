-- 0016_idempotency_retention_index.up.sql
BEGIN;

CREATE INDEX IF NOT EXISTS idempotency_keys_created_at_idx
  ON idempotency_keys(created_at);

COMMIT;
