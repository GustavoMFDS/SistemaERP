BEGIN;
-- Printer paper preference is operational data, scoped to each legal entity
-- and each physical cash register. Existing registers retain 80mm defaults.
ALTER TABLE cash_registers
  ADD COLUMN danfe_paper_width_mm smallint NOT NULL DEFAULT 80
  CONSTRAINT cash_registers_danfe_paper_width_check
  CHECK (danfe_paper_width_mm IN (58, 80));
COMMIT;
