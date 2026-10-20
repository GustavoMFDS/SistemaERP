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
    (return_policy = 'within_days' AND return_days IS NOT NULL AND return_days BETWEEN 1 AND 3650)
  );

-- Historical sales did not record this rule: NULL means unknown, not denied.
ALTER TABLE sale_items
  ADD COLUMN return_policy_snapshot text NULL,
  ADD COLUMN return_days_snapshot integer NULL;

ALTER TABLE sale_items ADD CONSTRAINT sale_items_return_policy_snapshot_check
  CHECK (
    CASE
      WHEN return_policy_snapshot IS NULL THEN return_days_snapshot IS NULL
      WHEN return_policy_snapshot IN ('allowed', 'denied') THEN return_days_snapshot IS NULL
      WHEN return_policy_snapshot = 'within_days' THEN
        return_days_snapshot IS NOT NULL AND return_days_snapshot BETWEEN 1 AND 3650
      ELSE FALSE
    END
  );
-- A privileged employee can document a protected consumer-rights case.
ALTER TABLE sale_returns
  ADD COLUMN return_basis text NOT NULL DEFAULT 'voluntary'
  CONSTRAINT sale_returns_basis_check CHECK (return_basis IN ('voluntary','consumer_rights'));

INSERT INTO permissions (id, code, description)
VALUES (gen_random_uuid(), 'sale:return:override', 'Analisar devolução por direito legal ou exceção')
ON CONFLICT (code) DO NOTHING;
INSERT INTO role_permissions(role_id, permission_id)
SELECT r.id, p.id
FROM roles r CROSS JOIN permissions p
WHERE r.name = 'admin' AND p.code = 'sale:return:override'
ON CONFLICT DO NOTHING;

COMMIT;
