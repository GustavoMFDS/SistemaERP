-- 0028_nfce_number_inutilizations.up.sql

BEGIN;

CREATE TABLE nfce_number_inutilizations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
  environment text NOT NULL,
  issuer_uf text NOT NULL,
  issuer_cnpj text NOT NULL,
  year integer NOT NULL,
  model smallint NOT NULL DEFAULT 65,
  series integer NOT NULL,
  start_number bigint NOT NULL,
  end_number bigint NOT NULL,
  request_id text NOT NULL,
  status text NOT NULL DEFAULT 'signed',
  justification text NOT NULL,
  signed_xml bytea NOT NULL,
  signed_sha256 text NOT NULL,
  response_xml bytea NULL,
  response_sha256 text NULL,
  status_code integer NULL,
  reason text NULL,
  protocol text NULL,
  registered_at timestamptz NULL,
  created_by_user_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT nfce_number_inutilizations_created_by_tenant_fk
    FOREIGN KEY (created_by_user_id, tenant_id)
    REFERENCES user_tenants(user_id, tenant_id)
    ON DELETE RESTRICT,

  CONSTRAINT nfce_number_inutilizations_environment_check
    CHECK (environment IN ('homologation','production')),

  CONSTRAINT nfce_number_inutilizations_uf_check
    CHECK (issuer_uf IN (
      'RO','AC','AM','RR','PA','AP','TO','MA','PI','CE','RN','PB','PE',
      'AL','SE','BA','MG','ES','RJ','SP','PR','SC','RS','MS','MT','GO','DF'
    )),

  CONSTRAINT nfce_number_inutilizations_cnpj_check
    CHECK (issuer_cnpj ~ '^[A-Z0-9]{12}[0-9]{2}$'),

  CONSTRAINT nfce_number_inutilizations_year_check
    CHECK (year BETWEEN 2006 AND 2099),

  CONSTRAINT nfce_number_inutilizations_model_check
    CHECK (model = 65),

  CONSTRAINT nfce_number_inutilizations_series_check
    CHECK (series BETWEEN 0 AND 889),

  CONSTRAINT nfce_number_inutilizations_range_check
    CHECK (
      start_number BETWEEN 1 AND 999999999
      AND end_number BETWEEN start_number AND 999999999
      AND end_number - start_number + 1 <= 10000
    ),

  CONSTRAINT nfce_number_inutilizations_request_id_check
    CHECK (request_id ~ '^ID[0-9]{4}[A-Z0-9]{12}[0-9]{25}$'),

  CONSTRAINT nfce_number_inutilizations_status_check
    CHECK (status IN ('signed','submitted','registered','rejected')),

  CONSTRAINT nfce_number_inutilizations_justification_check
    CHECK (char_length(btrim(justification)) BETWEEN 15 AND 255),

  CONSTRAINT nfce_number_inutilizations_signed_check
    CHECK (
      octet_length(signed_xml) > 0
      AND signed_sha256 ~ '^[0-9a-f]{64}$'
    ),

  CONSTRAINT nfce_number_inutilizations_response_check
    CHECK (
      status NOT IN ('registered','rejected')
      OR (
        octet_length(response_xml) > 0
        AND response_sha256 ~ '^[0-9a-f]{64}$'
        AND status_code IS NOT NULL
        AND NULLIF(btrim(reason), '') IS NOT NULL
      )
    ),

  CONSTRAINT nfce_number_inutilizations_registered_check
    CHECK (
      status <> 'registered'
      OR (
        status_code = 102
        AND NULLIF(btrim(protocol), '') IS NOT NULL
        AND registered_at IS NOT NULL
      )
    ),

  CONSTRAINT nfce_number_inutilizations_rejected_check
    CHECK (
      status <> 'rejected'
      OR (
        status_code <> 102
        AND protocol IS NULL
        AND registered_at IS NULL
      )
    ),

  CONSTRAINT nfce_number_inutilizations_tenant_request_unique
    UNIQUE (tenant_id, request_id)
);

CREATE INDEX nfce_number_inutilizations_tenant_range_idx
  ON nfce_number_inutilizations(
    tenant_id, environment, year, model, series, start_number, end_number
  );

CREATE OR REPLACE FUNCTION prevent_nfce_number_inutilization_delete()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  RAISE EXCEPTION 'NFC-e number inutilization records cannot be deleted';
END;
$$;

CREATE TRIGGER nfce_number_inutilizations_no_delete
BEFORE DELETE ON nfce_number_inutilizations
FOR EACH ROW
EXECUTE FUNCTION prevent_nfce_number_inutilization_delete();

COMMIT;
