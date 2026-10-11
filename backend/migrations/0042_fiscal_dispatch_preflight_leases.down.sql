BEGIN;
DO $guard$
BEGIN
 IF EXISTS (SELECT 1 FROM fiscal_dispatch_jobs WHERE claim_count<>0 OR lease_token IS NOT NULL LIMIT 1) THEN
   RAISE EXCEPTION 'Cannot drop 0042: fiscal preflight lease attempts are recorded';
 END IF;
END
$guard$;
DROP INDEX fiscal_dispatch_claim_due_idx;
ALTER TABLE fiscal_dispatch_jobs
 DROP CONSTRAINT fiscal_dispatch_result_code_check,
 DROP CONSTRAINT fiscal_dispatch_lease_consistency,
 DROP COLUMN lease_token,
 DROP COLUMN lease_owner,
 DROP COLUMN lease_until,
 DROP COLUMN claim_count,
 DROP COLUMN last_claimed_at,
 DROP COLUMN last_result_code,
 DROP COLUMN next_claim_at;
COMMIT;
