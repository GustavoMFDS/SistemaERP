BEGIN;
ALTER TABLE product_images
  ADD COLUMN caption text NOT NULL DEFAULT '';
ALTER TABLE product_images
  ADD CONSTRAINT product_images_caption_len_check
  CHECK (char_length(caption) <= 40);
COMMIT;
