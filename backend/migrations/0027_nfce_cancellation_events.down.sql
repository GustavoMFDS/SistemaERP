-- 0027_nfce_cancellation_events.down.sql

BEGIN;

DROP TRIGGER IF EXISTS invoice_fiscal_events_no_delete ON invoice_fiscal_events;
DROP FUNCTION IF EXISTS prevent_invoice_fiscal_event_delete();
DROP TABLE IF EXISTS invoice_fiscal_events;

ALTER TABLE invoices
  DROP CONSTRAINT IF EXISTS invoices_cancellation_fields_check,
  DROP COLUMN IF EXISTS cancellation_reason,
  DROP COLUMN IF EXISTS cancelled_at,
  DROP COLUMN IF EXISTS cancellation_protocol;

COMMIT;
