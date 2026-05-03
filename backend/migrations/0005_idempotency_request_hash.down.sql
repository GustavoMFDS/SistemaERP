-- 0005_idempotency_request_hash.down.sql

BEGIN;

DROP INDEX IF EXISTS idempotency_keys_tenant_op_key_hash_idx;

ALTER TABLE idempotency_keys
  DROP COLUMN IF EXISTS request_hash;

COMMIT;
