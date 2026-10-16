-- Return policy is a *voluntary shop policy*, never a substitute for legal rights.
-- Existing product policy defaults to the previously unrestricted behaviour.
BEGIN;

ALTER TABLE products
  ADD COLUMN return_policy text NOT NULL DEFAULT 'allowed',
  ADD COLUMN return_days integer NULL;

ALTER TABLE products ADD CONSTRAINT products_return_policy_check
  CHECK (
    (return_policy = 'allowed' AND return_days IS NULL) OR
    (return_policy = 'denied' AND return_days IS NULL) OR
    (return_policy = 'within_days' AND return_days BETWEEN 1 AND 3650)
  );

-- Historical sales did not record this rule: NULL means unknown, not denied.
ALTER TABLE sale_items
  ADD COLUMN return_policy_snapshot text NULL,
  ADD COLUMN return_days_snapshot integer NULL;

ALTER TABLE sale_items ADD CONSTRAINT sale_items_return_policy_snapshot_check
  CHECK (
    (return_policy_snapshot IS NULL AND return_days_snapshot IS NULL) OR
    (return_policy_snapshot IN ('allowed', 'denied') AND return_days_snapshot IS NULL) OR
    (return_policy_snapshot = 'within_days' AND return_days_snapshot BETWEEN 1 AND 3650)
  );
COMMIT;
