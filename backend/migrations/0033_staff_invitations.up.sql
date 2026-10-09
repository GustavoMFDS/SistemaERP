-- Staff membership is tenant-scoped. Never disable a shared user globally.
BEGIN;

ALTER TABLE user_tenants
  ADD COLUMN IF NOT EXISTS active boolean NOT NULL DEFAULT true;

INSERT INTO permissions(id, code, description)
VALUES (gen_random_uuid(), 'team:manage', 'Administrar equipe da propria empresa')
ON CONFLICT (code) DO NOTHING;

-- Only the pre-existing admin role can provision or change people.
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.code='team:manage'
WHERE r.name='admin'
ON CONFLICT DO NOTHING;

CREATE TABLE staff_invitations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  email citext NOT NULL,
  name text NOT NULL,
  role_name text NOT NULL CHECK (role_name IN ('cashier','manager')),
  token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash)=32),
  invited_by_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  accepted_at timestamptz NULL,
  revoked_at timestamptz NULL,
  CONSTRAINT staff_invite_state_check CHECK (accepted_at IS NULL OR revoked_at IS NULL)
);

CREATE UNIQUE INDEX staff_invite_one_pending_per_email
  ON staff_invitations(tenant_id, email)
  WHERE accepted_at IS NULL AND revoked_at IS NULL;
CREATE INDEX staff_invites_tenant_created_idx
  ON staff_invitations(tenant_id, created_at DESC);

COMMIT;
