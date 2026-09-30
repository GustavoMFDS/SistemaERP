-- 0023_nfce_foundation.down.sql

BEGIN;

ALTER TABLE invoice_xml_files
  DROP CONSTRAINT IF EXISTS invoice_xml_files_tenant_invoice_fk;

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_company_matches_tenant_check,
  DROP CONSTRAINT IF EXISTS invoices_tenant_sale_fk;

DROP INDEX IF EXISTS invoices_access_key_unique;
DROP INDEX IF EXISTS invoices_tenant_model_series_number_unique;
DROP INDEX IF EXISTS invoices_tenant_id_id_unique;

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_real_document_fields_check,
  DROP CONSTRAINT IF EXISTS invoices_status_check,
  DROP CONSTRAINT IF EXISTS invoices_access_key_check_digit_check,
  DROP CONSTRAINT IF EXISTS invoices_numeric_code_check,
  DROP CONSTRAINT IF EXISTS invoices_emission_type_check,
  DROP CONSTRAINT IF EXISTS invoices_access_key_check,
  DROP CONSTRAINT IF EXISTS invoices_environment_check,
  DROP CONSTRAINT IF EXISTS invoices_document_number_check,
  DROP CONSTRAINT IF EXISTS invoices_series_check,
  DROP CONSTRAINT IF EXISTS invoices_model_check;

ALTER TABLE invoices
  DROP COLUMN IF EXISTS rejection_message,
  DROP COLUMN IF EXISTS rejection_code,
  DROP COLUMN IF EXISTS authorized_at,
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS authorization_protocol,
  DROP COLUMN IF EXISTS issued_at,
  DROP COLUMN IF EXISTS access_key_check_digit,
  DROP COLUMN IF EXISTS numeric_code,
  DROP COLUMN IF EXISTS emission_type,
  DROP COLUMN IF EXISTS access_key,
  DROP COLUMN IF EXISTS environment,
  DROP COLUMN IF EXISTS document_number,
  DROP COLUMN IF EXISTS series,
  DROP COLUMN IF EXISTS model;

DROP TABLE IF EXISTS fiscal_document_sequences;
DROP TABLE IF EXISTS nfce_configs;

ALTER TABLE products
  DROP CONSTRAINT IF EXISTS products_cest_check,
  DROP CONSTRAINT IF EXISTS products_ncm_check,
  DROP COLUMN IF EXISTS cest,
  DROP COLUMN IF EXISTS ncm;

ALTER TABLE companies
  DROP CONSTRAINT IF EXISTS companies_address_city_code_check,
  DROP COLUMN IF EXISTS address_city_code;

COMMIT;
