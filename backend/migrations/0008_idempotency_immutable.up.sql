-- 0008_idempotency_immutable.up.sql

BEGIN;

CREATE OR REPLACE FUNCTION prevent_idempotency_key_update()
RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'idempotency records are immutable';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS idempotency_keys_immutable ON idempotency_keys;
CREATE TRIGGER idempotency_keys_immutable
  BEFORE UPDATE ON idempotency_keys
  FOR EACH ROW EXECUTE FUNCTION prevent_idempotency_key_update();

COMMIT;
