BEGIN;
-- Variants keep their own SKU, inventory balance, movement trail and fiscal profile.
-- This table only relates independently stocked products within one legal entity.
CREATE TABLE product_variations (
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  parent_product_id uuid NOT NULL,
  variant_product_id uuid NOT NULL,
  option_label varchar(40) NOT NULL CHECK (length(btrim(option_label)) BETWEEN 1 AND 40),
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT product_variations_not_self CHECK (parent_product_id <> variant_product_id),
  CONSTRAINT product_variations_variant_unique PRIMARY KEY (tenant_id, variant_product_id),
  CONSTRAINT product_variations_parent_fk FOREIGN KEY (tenant_id, parent_product_id)
    REFERENCES products(tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT product_variations_variant_fk FOREIGN KEY (tenant_id, variant_product_id)
    REFERENCES products(tenant_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX product_variations_parent_label_unique
  ON product_variations(tenant_id, parent_product_id, lower(option_label));
CREATE INDEX product_variations_parent_idx
  ON product_variations(tenant_id, parent_product_id, option_label);
COMMIT;
