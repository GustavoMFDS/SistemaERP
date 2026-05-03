-- 0008_idempotency_immutable.down.sql

BEGIN;

DROP TRIGGER IF EXISTS idempotency_keys_immutable ON idempotency_keys;
DROP FUNCTION IF EXISTS prevent_idempotency_key_update();

COMMIT;
