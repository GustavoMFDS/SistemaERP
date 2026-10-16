BEGIN;
DO $$
BEGIN
  IF EXISTS(SELECT 1 FROM nfce_auto_homologation_jobs LIMIT 1) THEN
    RAISE EXCEPTION 'cannot downgrade NFC-e auto-homologation with historical work';
  END IF;
END $$;
DROP TABLE nfce_auto_homologation_jobs;
COMMIT;
