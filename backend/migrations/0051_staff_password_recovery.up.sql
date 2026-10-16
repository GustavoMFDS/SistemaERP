-- Employee-controlled password recovery without e-mail. Issuance is restricted
-- to a tenant administrator; old JWTs are rejected by auth_version.
BEGIN;

ALTER TABLE users
  ADD COLUMN auth_version bigint NOT NULL DEFAULT 0
  CONSTRAINT users_auth_version_nonnegative CHECK (auth_version >= 0);

CREATE TABLE staff_password_resets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash)=32),
  requested_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  revoked_at timestamptz,
  CONSTRAINT staff_password_reset_state_check
    CHECK (consumed_at IS NULL OR revoked_at IS NULL),
  CONSTRAINT staff_password_reset_expiry_check CHECK (expires_at > created_at)
);

CREATE UNIQUE INDEX staff_password_reset_one_pending_per_user
  ON staff_password_resets(user_id)
  WHERE consumed_at IS NULL AND revoked_at IS NULL;
CREATE INDEX staff_password_reset_tenant_created_idx
  ON staff_password_resets(tenant_id,created_at DESC);

COMMIT;
