-- 0028_nfce_number_inutilizations.down.sql

BEGIN;

DROP TRIGGER IF EXISTS nfce_number_inutilizations_no_delete
  ON nfce_number_inutilizations;
DROP FUNCTION IF EXISTS prevent_nfce_number_inutilization_delete();
DROP TABLE IF EXISTS nfce_number_inutilizations;

COMMIT;
