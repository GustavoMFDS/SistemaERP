BEGIN;
DO $guard$
BEGIN
  IF EXISTS (SELECT 1 FROM payments WHERE cash_received IS NOT NULL OR cash_change IS NOT NULL) THEN
    RAISE EXCEPTION 'Cannot rollback cash tender migration while receipts exist';
  END IF;
END $guard$;
ALTER TABLE payments DROP CONSTRAINT payments_cash_tender_consistency;
ALTER TABLE payments DROP COLUMN cash_received, DROP COLUMN cash_change;
COMMIT;
