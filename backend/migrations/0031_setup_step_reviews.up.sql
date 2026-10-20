-- Persist only manual review acknowledgements. Automatic checks remain live API data.
BEGIN;
CREATE TABLE setup_step_reviews (
  tenant_id uuid NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
  step text NOT NULL CHECK (step IN ('stock', 'team')),
  reviewed_by_user_id uuid NOT NULL,
  reviewed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, step),
  CONSTRAINT setup_step_reviews_actor_tenant_fk
    FOREIGN KEY (reviewed_by_user_id, tenant_id)
    REFERENCES user_tenants(user_id, tenant_id)
    ON DELETE CASCADE
);
COMMIT;
