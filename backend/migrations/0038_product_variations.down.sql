BEGIN;
-- A rollback is safe only before real product families exist. Dropping links
-- once users have sold variants would silently erase the family history.
DO $guard$
BEGIN
  IF EXISTS (SELECT 1 FROM product_variations LIMIT 1) THEN
    RAISE EXCEPTION 'Cannot roll back 0038: product variations exist; preserve their links and audit history';
  END IF;
END
$guard$;
DROP TABLE IF EXISTS product_variations;
COMMIT;
