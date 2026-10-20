BEGIN;
-- Fail closed: never discard unknown SEFAZ outcomes by reverting a schema.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM nfce_consult_recovery_jobs LIMIT 1) THEN
    RAISE EXCEPTION 'cannot downgrade NFC-e consultation recovery with existing jobs';
  END IF;
END
$$;
DROP TABLE nfce_consult_recovery_jobs;
COMMIT;
