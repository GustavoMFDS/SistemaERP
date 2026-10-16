-- A username identifies exactly one user globally. It is never a
-- tenant-wide alias resolved without knowing which employee is logging in.
BEGIN;

ALTER TABLE users
  ADD COLUMN username citext NULL;
ALTER TABLE users
  ALTER COLUMN email DROP NOT NULL;
ALTER TABLE users
  ADD CONSTRAINT users_login_identity_present
  CHECK (email IS NOT NULL OR username IS NOT NULL);
ALTER TABLE users
  ADD CONSTRAINT users_username_format
  CHECK (username IS NULL OR username::text ~ '^[a-z][a-z0-9._-]{2,31}$');
CREATE UNIQUE INDEX users_username_global_unique
  ON users (username) WHERE username IS NOT NULL;

ALTER TABLE staff_invitations
  ADD COLUMN username citext NULL;
ALTER TABLE staff_invitations
  ALTER COLUMN email DROP NOT NULL;
ALTER TABLE staff_invitations
  ADD CONSTRAINT staff_invite_login_identity_present
  CHECK (email IS NOT NULL OR username IS NOT NULL);
ALTER TABLE staff_invitations
  ADD CONSTRAINT staff_invite_username_format
  CHECK (username IS NULL OR username::text ~ '^[a-z][a-z0-9._-]{2,31}$');
CREATE UNIQUE INDEX staff_invite_username_global_pending
  ON staff_invitations (username)
  WHERE username IS NOT NULL AND accepted_at IS NULL AND revoked_at IS NULL;

COMMIT;
