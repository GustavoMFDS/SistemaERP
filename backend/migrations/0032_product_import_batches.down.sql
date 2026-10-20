BEGIN;
-- Retain committed receipt history: rollback must not erase evidence.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM product_import_batches LIMIT 1) THEN
    RAISE EXCEPTION 'cannot downgrade product_import_batches: committed batches exist';
  END IF;
END
$$;
DROP TABLE product_import_batches;
COMMIT;
