BEGIN;
DO $guard$
BEGIN
 IF EXISTS(SELECT 1 FROM fiscal_dispatch_jobs WHERE state NOT IN ('pending','manual_review') LIMIT 1) THEN
  RAISE EXCEPTION 'Cannot drop fiscal dispatch history after reconciliation has started';
 END IF;
END
$guard$;
DROP TRIGGER fiscal_intent_enqueue_dispatch ON sale_fiscal_intents;
DROP FUNCTION enqueue_required_fiscal_dispatch();
DROP TABLE fiscal_dispatch_jobs;
COMMIT;
