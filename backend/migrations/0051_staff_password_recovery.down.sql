BEGIN;
-- Never lower token versions after a password change. It would resurrect
-- previously revoked access/refresh JWTs.
DO $guard$
BEGIN
  IF EXISTS (SELECT 1 FROM users WHERE auth_version <> 0)
    OR EXISTS (SELECT 1 FROM staff_password_resets) THEN
    RAISE EXCEPTION 'Cannot rollback password recovery with issued tokens or changed passwords';
  END IF;
END $guard$;

DROP TABLE staff_password_resets;
ALTER TABLE users DROP COLUMN auth_version;
COMMIT;
