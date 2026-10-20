package application

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReconcileNFCeArchivedIntegrity detects a previously completed automatic
// emission or consultation whose persisted authorization archive no longer
// satisfies the same tenant, access-key, protocol and SHA-256 requirements
// enforced by the DANFE/download routes, including unfinished queues
// after a crashed archive step. It changes only LOCAL queue state:
// no invoice is rejected, reissued or changed, and no SEFAZ request is made.
//
// A bad archive is an incident to investigate, not permission to resend.
// Live leases are never revoked by this background diagnostic.
func ReconcileNFCeArchivedIntegrity(
	ctx context.Context, pool *pgxpool.Pool,
) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	auto, err := tx.Exec(ctx, `
		UPDATE nfce_auto_homologation_jobs j SET
		  state='manual_review',last_step='manual_review',
		  lease_token=NULL,lease_owner=NULL,lease_until=NULL,
		  updated_at=now()
		FROM invoices i
		WHERE i.tenant_id=j.tenant_id AND i.sale_id=j.sale_id
		  AND i.model=65 AND i.status='authorized'
		  AND j.state IN ('authorized','queued','awaiting_sefaz')
		  AND (j.lease_until IS NULL OR j.lease_until<=clock_timestamp())
		  AND NOT (
		    length(btrim(COALESCE(i.authorization_protocol,'')))>0
		    AND i.authorized_at IS NOT NULL
		    AND EXISTS (
		      SELECT 1 FROM invoice_authorized_xml_files x
		      WHERE x.tenant_id=i.tenant_id AND x.invoice_id=i.id
		        AND x.access_key=i.access_key
		        AND octet_length(x.processed_xml)>0
		        AND octet_length(x.protocol_xml)>0
		        AND x.sha256=encode(digest(x.processed_xml,'sha256'),'hex')
		    )
		  )
	`)
	if err != nil {
		return 0, err
	}

	consult, err := tx.Exec(ctx, `
		UPDATE nfce_consult_recovery_jobs j SET
		  state='manual_review',last_result_code='manual_review',
		  lease_token=NULL,lease_owner=NULL,lease_until=NULL,
		  updated_at=now()
		FROM invoices i
		WHERE i.tenant_id=j.tenant_id AND i.id=j.invoice_id
		  AND i.model=65 AND i.status='authorized'
		  AND j.state IN ('resolved','awaiting_consultation')
		  AND (j.lease_until IS NULL OR j.lease_until<=clock_timestamp())
		  AND NOT (
		    length(btrim(COALESCE(i.authorization_protocol,'')))>0
		    AND i.authorized_at IS NOT NULL
		    AND i.access_key=j.access_key
		    AND EXISTS (
		      SELECT 1 FROM invoice_authorized_xml_files x
		      WHERE x.tenant_id=i.tenant_id AND x.invoice_id=i.id
		        AND x.access_key=i.access_key
		        AND octet_length(x.processed_xml)>0
		        AND octet_length(x.protocol_xml)>0
		        AND x.sha256=encode(digest(x.processed_xml,'sha256'),'hex')
		    )
		  )
	`)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return auto.RowsAffected() + consult.RowsAffected(), nil
}
