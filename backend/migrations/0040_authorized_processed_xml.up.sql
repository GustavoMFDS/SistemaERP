BEGIN;
-- The NFe signed payload and the authentic protNFe received from SEFAZ
-- must be stored atomically with authorization. A separate table prevents
-- technical unsigned/signed drafts being confused with a fiscal nfeProc.
CREATE TABLE invoice_authorized_xml_files (
  tenant_id uuid NOT NULL,
  invoice_id uuid NOT NULL,
  access_key text NOT NULL,
  file_name text NOT NULL,
  protocol_xml bytea NOT NULL CHECK (octet_length(protocol_xml) BETWEEN 1 AND 131072),
  processed_xml bytea NOT NULL CHECK (octet_length(processed_xml) BETWEEN 1 AND 2097152),
  sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, invoice_id),
  CONSTRAINT authorized_xml_invoice_fk
    FOREIGN KEY (tenant_id, invoice_id)
    REFERENCES invoices(tenant_id, id) ON DELETE RESTRICT,
  CONSTRAINT authorized_xml_filename_check
    CHECK (file_name = access_key || '-procNFe.xml'),
  CONSTRAINT authorized_xml_access_key_check
    CHECK (access_key ~ '^[0-9A-Z]{44}$')
);
CREATE UNIQUE INDEX authorized_xml_tenant_access_key_unique
 ON invoice_authorized_xml_files (tenant_id, access_key);
COMMIT;
