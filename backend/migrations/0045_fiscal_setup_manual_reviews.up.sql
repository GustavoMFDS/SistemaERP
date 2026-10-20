-- Extend the existing per-tenant audited manual setup checklist. These are
-- preparation acknowledgements, not SEFAZ approvals or fiscal authorization.
BEGIN;
ALTER TABLE setup_step_reviews
  DROP CONSTRAINT setup_step_reviews_step_check;
ALTER TABLE setup_step_reviews
  ADD CONSTRAINT setup_step_reviews_step_check CHECK (
    step IN (
      'stock','team',
      'fiscal_docs_collected',
      'fiscal_siare_access_arranged',
      'fiscal_accountant_consulted',
      'fiscal_previous_series_checked'
    )
  );
COMMIT;
