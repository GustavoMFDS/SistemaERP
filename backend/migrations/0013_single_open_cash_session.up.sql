-- 0013_single_open_cash_session.up.sql

BEGIN;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM cash_sessions
    WHERE status='open'
    GROUP BY tenant_id, cash_register_id
    HAVING count(*) > 1
  ) THEN
    RAISE EXCEPTION 'multiple open cash sessions exist for the same tenant/register; reconcile them before applying migration 0013';
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS cash_sessions_one_open_per_register
  ON cash_sessions(tenant_id, cash_register_id)
  WHERE status='open';

COMMIT;
