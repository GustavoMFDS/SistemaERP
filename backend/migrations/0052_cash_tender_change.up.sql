-- Cash tender is not revenue; existing cash payments without tender remain valid.
BEGIN;
ALTER TABLE payments ADD COLUMN cash_received numeric(12,2), ADD COLUMN cash_change numeric(12,2);
ALTER TABLE payments ADD CONSTRAINT payments_cash_tender_consistency CHECK (
  (method = 'cash' AND (
    (cash_received IS NULL AND cash_change IS NULL)
    OR (cash_received IS NOT NULL AND cash_change IS NOT NULL
        AND cash_received > 0 AND cash_received >= amount
        AND cash_change >= 0 AND cash_change = cash_received - amount)
  ))
  OR (method <> 'cash' AND cash_received IS NULL AND cash_change IS NULL)
);
COMMIT;
