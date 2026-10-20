BEGIN;
ALTER TABLE accounts_payable
  ADD COLUMN IF NOT EXISTS creation_key uuid,
  ADD COLUMN IF NOT EXISTS request_hash text,
  ADD COLUMN IF NOT EXISTS created_by_user_id uuid REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS settled_at timestamptz,
  ADD COLUMN IF NOT EXISTS settled_by_user_id uuid REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS settlement_key uuid,
  ADD COLUMN IF NOT EXISTS settlement_method text,
  ADD COLUMN IF NOT EXISTS settlement_note text;
ALTER TABLE accounts_receivable
  ADD COLUMN IF NOT EXISTS creation_key uuid,
  ADD COLUMN IF NOT EXISTS request_hash text,
  ADD COLUMN IF NOT EXISTS created_by_user_id uuid REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS settled_at timestamptz,
  ADD COLUMN IF NOT EXISTS settled_by_user_id uuid REFERENCES users(id),
  ADD COLUMN IF NOT EXISTS settlement_key uuid,
  ADD COLUMN IF NOT EXISTS settlement_method text,
  ADD COLUMN IF NOT EXISTS settlement_note text;
CREATE UNIQUE INDEX IF NOT EXISTS ap_creation_key_tenant_uq ON accounts_payable(tenant_id, creation_key) WHERE creation_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ar_creation_key_tenant_uq ON accounts_receivable(tenant_id, creation_key) WHERE creation_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ap_settlement_key_tenant_uq ON accounts_payable(tenant_id, settlement_key) WHERE settlement_key IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ar_settlement_key_tenant_uq ON accounts_receivable(tenant_id, settlement_key) WHERE settlement_key IS NOT NULL;
ALTER TABLE accounts_payable ADD CONSTRAINT ap_settlement_method_check CHECK (settlement_method IS NULL OR settlement_method IN ('cash','pix','debit','credit','transfer','other'));
ALTER TABLE accounts_receivable ADD CONSTRAINT ar_settlement_method_check CHECK (settlement_method IS NULL OR settlement_method IN ('cash','pix','debit','credit','transfer','other'));
COMMIT;
