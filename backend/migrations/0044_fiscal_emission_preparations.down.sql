BEGIN;
DO $guard$
BEGIN
 IF EXISTS(SELECT 1 FROM fiscal_emission_preparations LIMIT 1) THEN
  RAISE EXCEPTION 'cannot discard fiscal preparation snapshots';
 END IF;
END
$guard$;
DROP TRIGGER fiscal_emission_preparations_immutable ON fiscal_emission_preparations;
DROP FUNCTION prevent_fiscal_emission_preparation_mutation();
DROP TABLE fiscal_emission_preparations;
COMMIT;
