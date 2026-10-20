-- 0029_nfce_offline_contingency.up.sql

BEGIN;

ALTER TABLE invoices
  ADD COLUMN IF NOT EXISTS contingency_started_at timestamptz NULL,
  ADD COLUMN IF NOT EXISTS contingency_reason text NULL;

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_contingency_metadata_check;
ALTER TABLE invoices
  ADD CONSTRAINT invoices_contingency_metadata_check
  CHECK (
    (
      emission_type = 9
      AND contingency_started_at IS NOT NULL
      AND contingency_started_at <= issued_at
      AND char_length(btrim(contingency_reason)) BETWEEN 15 AND 256
    )
    OR (
      emission_type IS DISTINCT FROM 9
      AND contingency_started_at IS NULL
      AND contingency_reason IS NULL
    )
  );

COMMIT;
