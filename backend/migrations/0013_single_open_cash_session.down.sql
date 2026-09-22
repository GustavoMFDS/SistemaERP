-- 0013_single_open_cash_session.down.sql

BEGIN;
DROP INDEX IF EXISTS cash_sessions_one_open_per_register;
COMMIT;
