BEGIN;
ALTER TABLE product_images DROP CONSTRAINT IF EXISTS product_images_caption_len_check;
ALTER TABLE product_images DROP COLUMN IF EXISTS caption;
COMMIT;
