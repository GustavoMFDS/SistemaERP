BEGIN;
DO $guard$
BEGIN
 IF EXISTS (SELECT 1 FROM invoice_authorized_xml_files LIMIT 1) THEN
  RAISE EXCEPTION 'Cannot roll back 0040: authorized processed XMLs must not be discarded';
 END IF;
END
$guard$;
DROP TABLE invoice_authorized_xml_files;
COMMIT;
