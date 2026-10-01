-- 0024_nfce_tax_profiles.up.sql

BEGIN;

-- Composite keys used to enforce tenant isolation in the fiscal profile/snapshot layer.
CREATE UNIQUE INDEX IF NOT EXISTS products_tenant_id_id_unique
  ON products(tenant_id, id);

CREATE UNIQUE INDEX IF NOT EXISTS sale_items_tenant_id_id_unique
  ON sale_items(tenant_id, id);

-- Current fiscal classification for a product. Values here describe how a product
-- should be treated when an NFC-e reservation is created. They are copied into
-- immutable sale-item snapshots so later catalog edits never rewrite history.
CREATE TABLE IF NOT EXISTS product_fiscal_profiles (
  tenant_id uuid NOT NULL,
  product_id uuid NOT NULL,
  cfop text NOT NULL,
  icms_origin text NOT NULL,
  icms_regime text NOT NULL,
  icms_code text NOT NULL,
  pis_cst text NOT NULL,
  cofins_cst text NOT NULL,
  ibs_cbs_cst text NULL,
  ibs_cbs_classification text NULL,
  is_cst text NULL,
  is_classification text NULL,
  reference_version text NOT NULL,
  updated_by_user_id uuid NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  PRIMARY KEY (tenant_id, product_id),

  CONSTRAINT product_fiscal_profiles_product_fk
    FOREIGN KEY (tenant_id, product_id)
    REFERENCES products(tenant_id, id)
    ON DELETE CASCADE,

  CONSTRAINT product_fiscal_profiles_updated_by_tenant_fk
    FOREIGN KEY (updated_by_user_id, tenant_id)
    REFERENCES user_tenants(user_id, tenant_id)
    ON DELETE SET NULL (updated_by_user_id),

  CONSTRAINT product_fiscal_profiles_cfop_check
    CHECK (cfop ~ '^[0-9]{4}$'),

  CONSTRAINT product_fiscal_profiles_icms_origin_check
    CHECK (icms_origin ~ '^[0-8]$'),

  CONSTRAINT product_fiscal_profiles_icms_regime_check
    CHECK (icms_regime IN ('cst','csosn')),

  CONSTRAINT product_fiscal_profiles_icms_code_check
    CHECK (
      (icms_regime='cst' AND icms_code ~ '^[0-9]{2}$')
      OR
      (icms_regime='csosn' AND icms_code ~ '^[0-9]{3}$')
    ),

  CONSTRAINT product_fiscal_profiles_pis_cst_check
    CHECK (pis_cst ~ '^[0-9]{2}$'),

  CONSTRAINT product_fiscal_profiles_cofins_cst_check
    CHECK (cofins_cst ~ '^[0-9]{2}$'),

  CONSTRAINT product_fiscal_profiles_ibs_cbs_check
    CHECK (
      (ibs_cbs_cst IS NULL AND ibs_cbs_classification IS NULL)
      OR (
        ibs_cbs_cst ~ '^[0-9]{3}$'
        AND ibs_cbs_classification ~ '^[0-9]{6}$'
        AND left(ibs_cbs_classification, 3) = ibs_cbs_cst
      )
    ),

  CONSTRAINT product_fiscal_profiles_is_check
    CHECK (
      (is_cst IS NULL AND is_classification IS NULL)
      OR (
        is_cst ~ '^[0-9]{3}$'
        AND is_classification ~ '^[0-9]{6}$'
      )
    ),

  CONSTRAINT product_fiscal_profiles_reference_version_check
    CHECK (NULLIF(btrim(reference_version), '') IS NOT NULL)
);

-- Classification snapshot captured at fiscal reservation time. Tax calculation
-- is intentionally kept separate because it depends on the invoice operation,
-- current legal tables and calculated monetary values.
CREATE TABLE IF NOT EXISTS sale_item_fiscal_snapshots (
  tenant_id uuid NOT NULL,
  sale_item_id uuid NOT NULL,
  sale_id uuid NOT NULL,
  product_id uuid NOT NULL,

  ncm text NOT NULL,
  cest text NULL,
  cfop text NOT NULL,
  icms_origin text NOT NULL,
  icms_regime text NOT NULL,
  icms_code text NOT NULL,
  pis_cst text NOT NULL,
  cofins_cst text NOT NULL,
  ibs_cbs_cst text NULL,
  ibs_cbs_classification text NULL,
  is_cst text NULL,
  is_classification text NULL,
  reference_version text NOT NULL,

  snapshot_sha256 text NOT NULL,
  captured_at timestamptz NOT NULL DEFAULT now(),

  PRIMARY KEY (tenant_id, sale_item_id),

  CONSTRAINT sale_item_fiscal_snapshots_sale_item_fk
    FOREIGN KEY (tenant_id, sale_item_id)
    REFERENCES sale_items(tenant_id, id)
    ON DELETE CASCADE,

  CONSTRAINT sale_item_fiscal_snapshots_product_fk
    FOREIGN KEY (tenant_id, product_id)
    REFERENCES products(tenant_id, id)
    ON DELETE RESTRICT,

  CONSTRAINT sale_item_fiscal_snapshots_ncm_check
    CHECK (ncm ~ '^[0-9]{8}$'),

  CONSTRAINT sale_item_fiscal_snapshots_cest_check
    CHECK (cest IS NULL OR cest ~ '^[0-9]{7}$'),

  CONSTRAINT sale_item_fiscal_snapshots_cfop_check
    CHECK (cfop ~ '^[0-9]{4}$'),

  CONSTRAINT sale_item_fiscal_snapshots_icms_origin_check
    CHECK (icms_origin ~ '^[0-8]$'),

  CONSTRAINT sale_item_fiscal_snapshots_icms_regime_check
    CHECK (icms_regime IN ('cst','csosn')),

  CONSTRAINT sale_item_fiscal_snapshots_icms_code_check
    CHECK (
      (icms_regime='cst' AND icms_code ~ '^[0-9]{2}$')
      OR
      (icms_regime='csosn' AND icms_code ~ '^[0-9]{3}$')
    ),

  CONSTRAINT sale_item_fiscal_snapshots_pis_cst_check
    CHECK (pis_cst ~ '^[0-9]{2}$'),

  CONSTRAINT sale_item_fiscal_snapshots_cofins_cst_check
    CHECK (cofins_cst ~ '^[0-9]{2}$'),

  CONSTRAINT sale_item_fiscal_snapshots_ibs_cbs_check
    CHECK (
      (ibs_cbs_cst IS NULL AND ibs_cbs_classification IS NULL)
      OR (
        ibs_cbs_cst ~ '^[0-9]{3}$'
        AND ibs_cbs_classification ~ '^[0-9]{6}$'
        AND left(ibs_cbs_classification, 3) = ibs_cbs_cst
      )
    ),

  CONSTRAINT sale_item_fiscal_snapshots_is_check
    CHECK (
      (is_cst IS NULL AND is_classification IS NULL)
      OR (
        is_cst ~ '^[0-9]{3}$'
        AND is_classification ~ '^[0-9]{6}$'
      )
    ),

  CONSTRAINT sale_item_fiscal_snapshots_reference_version_check
    CHECK (NULLIF(btrim(reference_version), '') IS NOT NULL),

  CONSTRAINT sale_item_fiscal_snapshots_tax_calculation_object_check
    CHECK (jsonb_typeof(tax_calculation) = 'object'),

  CONSTRAINT sale_item_fiscal_snapshots_sha256_check
    CHECK (snapshot_sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX IF NOT EXISTS sale_item_fiscal_snapshots_tenant_sale_idx
  ON sale_item_fiscal_snapshots(tenant_id, sale_id);

COMMIT;
