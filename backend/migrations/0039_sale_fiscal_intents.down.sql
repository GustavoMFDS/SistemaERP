BEGIN;
-- Fail closed: dropping an unfulfilled fiscal obligation would destroy the
-- audit trail for sales made after this migration.
DO $guard$
BEGIN
  IF EXISTS (SELECT 1 FROM sale_fiscal_intents WHERE legacy_review = false LIMIT 1) THEN
    RAISE EXCEPTION 'Cannot roll back 0039: committed sales have mandatory fiscal intents';
  END IF;
END
$guard$;
DROP TRIGGER sales_require_fiscal_intent ON sales;
DROP FUNCTION record_required_sale_fiscal_intent();
DROP TABLE sale_fiscal_intents;
COMMIT;
