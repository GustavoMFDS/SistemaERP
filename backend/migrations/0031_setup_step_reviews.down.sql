BEGIN;
-- Avoid silently discarding review history during an accidental rollback.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM setup_step_reviews LIMIT 1) THEN
    RAISE EXCEPTION 'cannot downgrade setup_step_reviews: review records exist';
  END IF;
END
$$;
DROP TABLE setup_step_reviews;
COMMIT;
