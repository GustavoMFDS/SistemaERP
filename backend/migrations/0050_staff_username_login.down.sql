BEGIN;
-- Do not erase or misidentify accounts created without an e-mail address.
-- Restore e-mail-only schema only after every such account/invite has been
-- migrated to a real verified e-mail or appropriately deactivated and deleted.
DO $guard$
BEGIN
  IF EXISTS (SELECT 1 FROM users WHERE email IS NULL)
     OR EXISTS (SELECT 1 FROM staff_invitations WHERE email IS NULL) THEN
    RAISE EXCEPTION 'Cannot rollback username login: email-less accounts/invitations still exist';
  END IF;
END $guard$;

DROP INDEX staff_invite_username_global_pending;
ALTER TABLE staff_invitations DROP CONSTRAINT staff_invite_username_format;
ALTER TABLE staff_invitations DROP CONSTRAINT staff_invite_login_identity_present;
ALTER TABLE staff_invitations ALTER COLUMN email SET NOT NULL;
ALTER TABLE staff_invitations DROP COLUMN username;

DROP INDEX users_username_global_unique;
ALTER TABLE users DROP CONSTRAINT users_username_format;
ALTER TABLE users DROP CONSTRAINT users_login_identity_present;
ALTER TABLE users ALTER COLUMN email SET NOT NULL;
ALTER TABLE users DROP COLUMN username;
COMMIT;
