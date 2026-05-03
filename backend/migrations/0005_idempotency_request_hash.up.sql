-- 0005_idempotency_request_hash.up.sql

BEGIN;

ALTER TABLE idempotency_keys
  ADD COLUMN IF NOT EXISTS request_hash text;

UPDATE idempotency_keys
SET request_hash = ''
WHERE request_hash IS NULL;

ALTER TABLE idempotency_keys
  ALTER COLUMN request_hash SET NOT NULL;

CREATE INDEX IF NOT EXISTS idempotency_keys_tenant_op_key_hash_idx
  ON idempotency_keys(tenant_id, operation, idem_key, request_hash);

COMMIT;
