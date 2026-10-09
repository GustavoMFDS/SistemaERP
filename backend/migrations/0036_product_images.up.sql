BEGIN;

-- Images are always owned by the same independent store as their product.
-- A composite FK prevents cross-company associations even if a caller bypasses the API.
CREATE UNIQUE INDEX IF NOT EXISTS products_tenant_identity_uq ON products(tenant_id, id);

CREATE TABLE product_images (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  product_id uuid NOT NULL,
  upload_key uuid NOT NULL,
  content_sha256 bytea NOT NULL CHECK (octet_length(content_sha256) = 32),
  media_type text NOT NULL DEFAULT 'image/jpeg' CHECK (media_type = 'image/jpeg'),
  image_data bytea NOT NULL CHECK (octet_length(image_data) BETWEEN 1 AND 262144),
  thumb_data bytea NOT NULL CHECK (octet_length(thumb_data) BETWEEN 1 AND 12288),
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT product_images_product_tenant_fk
    FOREIGN KEY (tenant_id, product_id) REFERENCES products(tenant_id, id) ON DELETE CASCADE,
  CONSTRAINT product_images_idempotency_uq UNIQUE (tenant_id, product_id, upload_key)
);
CREATE INDEX product_images_tenant_product_order_idx
  ON product_images(tenant_id, product_id, created_at, id);

COMMIT;
