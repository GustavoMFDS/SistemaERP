BEGIN;
-- Fail closed: reversing the schema must not erase/reinterpret checked
-- fiscal-preparation tasks. Explicitly reopen/clear them before rollback.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM setup_step_reviews WHERE step NOT IN ('stock','team')
  ) THEN
    RAISE EXCEPTION 'cannot downgrade fiscal setup reviews: fiscal records exist';
  END IF;
END $$;
ALTER TABLE setup_step_reviews
  DROP CONSTRAINT setup_step_reviews_step_check;
ALTER TABLE setup_step_reviews
  ADD CONSTRAINT setup_step_reviews_step_check CHECK (step IN ('stock','team'));
COMMIT;
