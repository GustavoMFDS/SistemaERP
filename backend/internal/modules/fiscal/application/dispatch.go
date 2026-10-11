package application

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReconcileFiscalDispatchBatch classifies durable fiscal obligations without
// reserving numbers, signing XML, or contacting SEFAZ. Concurrent replicas
// use SKIP LOCKED to avoid processing the same row at the same time.
func ReconcileFiscalDispatchBatch(ctx context.Context, pool *pgxpool.Pool, limit int32) (int64, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	tag, err := pool.Exec(ctx, `
 WITH picked AS (
   SELECT j.tenant_id,j.sale_id
   FROM fiscal_dispatch_jobs j
   WHERE j.next_check_at <= now()
   ORDER BY j.next_check_at,j.tenant_id,j.sale_id
   LIMIT $1
   FOR UPDATE OF j SKIP LOCKED
 ), classified AS (
   SELECT k.tenant_id,k.sale_id,
    CASE
     WHEN f.legacy_review THEN 'manual_review'
     WHEN s.status='cancelled' THEN 'cancelled_review'
     WHEN s.status <> 'finalized' THEN 'waiting_sale'
     WHEN f.document_kind <> 'nfce' THEN 'unsupported_model'
     WHEN i.id IS NULL
      AND (cfg.tenant_id IS NULL OR NULLIF(btrim(cfg.certificate_secret_ref),'') IS NULL)
      THEN 'awaiting_configuration'
     WHEN i.id IS NULL THEN 'awaiting_fiscal_processing'
     WHEN i.model<>65 OR i.model IS NULL THEN 'requires_review'
     WHEN i.status='authorized'
       AND p.invoice_id IS NOT NULL
       AND i.authorization_protocol IS NOT NULL
       AND btrim(i.authorization_protocol)<>''
       AND i.authorized_at IS NOT NULL
       AND i.access_key=p.access_key
       THEN 'authorized'
     WHEN i.status='authorized' THEN 'missing_processed_xml'
     WHEN i.status='reserved' THEN 'reserved'
     WHEN i.status='xml_generated' THEN 'xml_generated'
     WHEN i.status='signed' THEN 'signed'
     WHEN i.status='submitted' THEN 'awaiting_sefaz'
     WHEN i.status='rejected' THEN 'rejected_review'
     WHEN i.status='cancelled' THEN 'cancelled_review'
     ELSE 'requires_review'
    END AS new_state
   FROM picked k
   JOIN sale_fiscal_intents f ON f.tenant_id=k.tenant_id AND f.sale_id=k.sale_id
   JOIN sales s ON s.tenant_id=k.tenant_id AND s.id=k.sale_id
   LEFT JOIN nfce_configs cfg ON cfg.tenant_id=k.tenant_id
   LEFT JOIN LATERAL (
      SELECT i.id,i.model,i.status,i.authorization_protocol,
        i.authorized_at,i.access_key
      FROM invoices i
      WHERE i.tenant_id=k.tenant_id AND i.sale_id=k.sale_id
      ORDER BY i.created_at DESC,i.id DESC LIMIT 1
   ) i ON true
   LEFT JOIN invoice_authorized_xml_files p
     ON p.tenant_id=k.tenant_id AND p.invoice_id=i.id
 )
 UPDATE fiscal_dispatch_jobs j
 SET state=c.new_state,check_count=j.check_count+1,
     last_checked_at=now(),
     next_check_at=now()+CASE
       WHEN c.new_state IN ('awaiting_sefaz','signed','reserved','xml_generated')
         THEN interval '1 minute'
       WHEN c.new_state='authorized' THEN interval '30 minutes'
       WHEN c.new_state IN ('manual_review','unsupported_model','cancelled_review')
         THEN interval '1 hour'
       ELSE interval '10 minutes'
     END
 FROM classified c
 WHERE j.tenant_id=c.tenant_id AND j.sale_id=c.sale_id
 `, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// StartFiscalDispatchReconciliation performs LOCAL read-only classification
// (aside from dispatch status rows). It never contacts fiscal authorities.
func StartFiscalDispatchReconciliation(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	run := func() {
		tickCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		for batch := 0; batch < 3; batch++ {
			n, err := ReconcileFiscalDispatchBatch(tickCtx, pool, 100)
			if err != nil {
				if tickCtx.Err() == nil {
					logger.Warn("fiscal_dispatch_reconcile_failed", slog.Any("err", err))
				}
				return
			}
			if n < 100 {
				return
			}
		}
	}
	go func() {
		run()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}
