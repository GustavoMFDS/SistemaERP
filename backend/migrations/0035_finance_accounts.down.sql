BEGIN;
ALTER TABLE accounts_payable DROP CONSTRAINT IF EXISTS ap_settlement_method_check;
ALTER TABLE accounts_receivable DROP CONSTRAINT IF EXISTS ar_settlement_method_check;
DROP INDEX IF EXISTS ap_creation_key_tenant_uq;
DROP INDEX IF EXISTS ar_creation_key_tenant_uq;
DROP INDEX IF EXISTS ap_settlement_key_tenant_uq;
DROP INDEX IF EXISTS ar_settlement_key_tenant_uq;
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM accounts_payable WHERE settlement_key IS NOT NULL OR creation_key IS NOT NULL)
     OR EXISTS (SELECT 1 FROM accounts_receivable WHERE settlement_key IS NOT NULL OR creation_key IS NOT NULL) THEN
    RAISE EXCEPTION 'unsafe rollback: financial account provenance/settlements would be lost';
  END IF;
END $$;
ALTER TABLE accounts_payable DROP COLUMN IF EXISTS creation_key, DROP COLUMN IF EXISTS request_hash,
  DROP COLUMN IF EXISTS created_by_user_id, DROP COLUMN IF EXISTS settled_at,
  DROP COLUMN IF EXISTS settled_by_user_id, DROP COLUMN IF EXISTS settlement_key,
  DROP COLUMN IF EXISTS settlement_method, DROP COLUMN IF EXISTS settlement_note;
ALTER TABLE accounts_receivable DROP COLUMN IF EXISTS creation_key, DROP COLUMN IF EXISTS request_hash,
  DROP COLUMN IF EXISTS created_by_user_id, DROP COLUMN IF EXISTS settled_at,
  DROP COLUMN IF EXISTS settled_by_user_id, DROP COLUMN IF EXISTS settlement_key,
  DROP COLUMN IF EXISTS settlement_method, DROP COLUMN IF EXISTS settlement_note;
COMMIT;
