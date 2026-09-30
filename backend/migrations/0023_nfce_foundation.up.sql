-- 0023_nfce_foundation.up.sql

BEGIN;

-- Municipality IBGE code is required to identify the issuer address in NF-e/NFC-e.
ALTER TABLE companies
  ADD COLUMN IF NOT EXISTS address_city_code text NULL;

ALTER TABLE companies
  DROP CONSTRAINT IF EXISTS companies_address_city_code_check;
ALTER TABLE companies
  ADD CONSTRAINT companies_address_city_code_check
  CHECK (
    address_city_code IS NULL
    OR address_city_code ~ '^[0-9]{7}$'
  );

-- Product fiscal classification. CFOP is intentionally NOT stored here because
-- it depends on the fiscal operation, not only on the catalog item.
ALTER TABLE products
  ADD COLUMN IF NOT EXISTS ncm text NULL,
  ADD COLUMN IF NOT EXISTS cest text NULL;

ALTER TABLE products
  DROP CONSTRAINT IF EXISTS products_ncm_check;
ALTER TABLE products
  ADD CONSTRAINT products_ncm_check
  CHECK (ncm IS NULL OR ncm ~ '^[0-9]{8}$');

ALTER TABLE products
  DROP CONSTRAINT IF EXISTS products_cest_check;
ALTER TABLE products
  ADD CONSTRAINT products_cest_check
  CHECK (cest IS NULL OR cest ~ '^[0-9]{7}$');

-- NFC-e configuration is tenant-scoped. Secrets are never stored here:
-- certificate_secret_ref and csc_secret_ref point to the deployment secret store.
CREATE TABLE IF NOT EXISTS nfce_configs (
  tenant_id uuid PRIMARY KEY REFERENCES companies(id) ON DELETE CASCADE,
  enabled boolean NOT NULL DEFAULT false,
  environment text NOT NULL DEFAULT 'homologation',
  series integer NOT NULL DEFAULT 1,
  csc_id text NULL,
  csc_secret_ref text NULL,
  certificate_secret_ref text NULL,
  updated_by_user_id uuid NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT nfce_configs_environment_check
    CHECK (environment IN ('homologation','production')),
  CONSTRAINT nfce_configs_series_check
    CHECK (series BETWEEN 0 AND 889),
  CONSTRAINT nfce_configs_updated_by_tenant_fk
    FOREIGN KEY (updated_by_user_id, tenant_id)
    REFERENCES user_tenants(user_id, tenant_id)
    ON DELETE SET NULL (updated_by_user_id),
  CONSTRAINT nfce_configs_enabled_secrets_check
    CHECK (
      NOT enabled
      OR (
        NULLIF(btrim(csc_id), '') IS NOT NULL
        AND NULLIF(btrim(csc_secret_ref), '') IS NOT NULL
        AND NULLIF(btrim(certificate_secret_ref), '') IS NOT NULL
      )
    )
);

-- Sequence ownership lives in PostgreSQL and is locked transactionally by the
-- future SEFAZ provider. Model 65 is the only model enabled by this foundation.
CREATE TABLE IF NOT EXISTS fiscal_document_sequences (
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  model smallint NOT NULL,
  series integer NOT NULL,
  next_number bigint NOT NULL DEFAULT 1,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, model, series),
  CONSTRAINT fiscal_document_sequences_model_check CHECK (model = 65),
  CONSTRAINT fiscal_document_sequences_series_check CHECK (series BETWEEN 0 AND 889),
  CONSTRAINT fiscal_document_sequences_number_check CHECK (next_number BETWEEN 1 AND 1000000000)
);

ALTER TABLE invoices
  ADD COLUMN IF NOT EXISTS model smallint NULL,
  ADD COLUMN IF NOT EXISTS series integer NULL,
  ADD COLUMN IF NOT EXISTS document_number bigint NULL,
  ADD COLUMN IF NOT EXISTS environment text NULL,
  ADD COLUMN IF NOT EXISTS access_key text NULL,
  ADD COLUMN IF NOT EXISTS emission_type smallint NULL,
  ADD COLUMN IF NOT EXISTS numeric_code text NULL,
  ADD COLUMN IF NOT EXISTS access_key_check_digit smallint NULL,
  ADD COLUMN IF NOT EXISTS issued_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS authorization_protocol text NULL,
  ADD COLUMN IF NOT EXISTS authorized_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS rejection_code text NULL,
  ADD COLUMN IF NOT EXISTS rejection_message text NULL,
  ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_model_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_model_check
  CHECK (model IS NULL OR model = 65);

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_series_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_series_check
  CHECK (series IS NULL OR series BETWEEN 0 AND 889);

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_document_number_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_document_number_check
  CHECK (document_number IS NULL OR document_number BETWEEN 1 AND 999999999);

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_environment_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_environment_check
  CHECK (environment IS NULL OR environment IN ('homologation','production'));

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_access_key_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_access_key_check
  -- DFe access keys remain 44 characters; the 12 CNPJ body positions can now be
  -- alphanumeric while every other position remains numeric.
  CHECK (access_key IS NULL OR access_key ~ '^[0-9]{6}[A-Z0-9]{12}[0-9]{26}$');

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_emission_type_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_emission_type_check
  CHECK (emission_type IS NULL OR emission_type BETWEEN 1 AND 9);

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_numeric_code_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_numeric_code_check
  CHECK (numeric_code IS NULL OR numeric_code ~ '^[0-9]{8}$');

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_access_key_check_digit_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_access_key_check_digit_check
  CHECK (access_key_check_digit IS NULL OR access_key_check_digit BETWEEN 0 AND 9);

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_status_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_status_check
  CHECK (
    status IN (
      'xml_generated',
      'reserved',
      'signed',
      'submitted',
      'authorized',
      'rejected',
      'contingency_pending',
      'cancelled'
    )
  );

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_real_document_fields_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_real_document_fields_check
  CHECK (
    status = 'xml_generated'
    OR (
      model = 65
      AND series IS NOT NULL
      AND document_number IS NOT NULL
      AND environment IS NOT NULL
      AND access_key IS NOT NULL
      AND emission_type IS NOT NULL
      AND numeric_code IS NOT NULL
      AND access_key_check_digit IS NOT NULL
      AND issued_at IS NOT NULL
    )
  );

CREATE UNIQUE INDEX IF NOT EXISTS invoices_tenant_model_series_number_unique
  ON invoices(tenant_id, model, series, document_number)
  WHERE model IS NOT NULL
    AND series IS NOT NULL
    AND document_number IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS invoices_access_key_unique
  ON invoices(access_key)
  WHERE access_key IS NOT NULL;

COMMIT;
