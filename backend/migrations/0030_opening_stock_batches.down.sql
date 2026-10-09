-- 0030_opening_stock_batches.down.sql
BEGIN;
-- Do not cascade or erase movement history when reversing a pilot migration.
-- A downgrade is blocked if a stock opening batch has been committed.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM opening_stock_batches LIMIT 1) THEN
    RAISE EXCEPTION 'cannot downgrade opening_stock_batches: committed batches exist';
  END IF;
END
$$;
DROP TABLE IF EXISTS opening_stock_batches;
COMMIT;
