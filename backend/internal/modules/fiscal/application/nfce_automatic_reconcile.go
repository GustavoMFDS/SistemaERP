package application

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReconcileNFCeAutomaticHomologationJobs repairs the local queue after
// interruptions, without signing, issuing or talking to SEFAZ.
func ReconcileNFCeAutomaticHomologationJobs(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	submitted, err := tx.Exec(ctx, `
		UPDATE nfce_auto_homologation_jobs j SET
		  state='awaiting_sefaz',last_step='awaiting_sefaz',
		  lease_token=NULL,lease_owner=NULL,lease_until=NULL,updated_at=now()
		FROM invoices i
		WHERE i.tenant_id=j.tenant_id AND i.sale_id=j.sale_id
		  AND i.model=65 AND i.environment='homologation'
		  AND i.status='submitted' AND j.state='queued'
		  AND (j.lease_until IS NULL OR j.lease_until<=now())
	`)
	if err != nil {
		return 0, err
	}

	finalized, err := tx.Exec(ctx, `
		UPDATE nfce_auto_homologation_jobs j SET
		  state='manual_review',last_step='manual_review',
		  lease_token=NULL,lease_owner=NULL,lease_until=NULL,updated_at=now()
		FROM invoices i
		WHERE i.tenant_id=j.tenant_id AND i.sale_id=j.sale_id
		  AND i.model=65 AND i.status IN ('rejected','cancelled')
		  AND j.state IN ('queued','awaiting_sefaz')
		  AND (j.lease_until IS NULL OR j.lease_until<=now())
	`)
	if err != nil {
		return 0, err
	}

	exhausted, err := tx.Exec(ctx, `
		UPDATE nfce_auto_homologation_jobs j SET
		  state='manual_review',last_step='manual_review',
		  lease_token=NULL,lease_owner=NULL,lease_until=NULL,updated_at=now()
		WHERE j.state='queued' AND j.attempt_count>=12
		  AND (j.lease_until IS NULL OR j.lease_until<=now())
		  AND NOT EXISTS (
		    SELECT 1 FROM invoices i
		    WHERE i.tenant_id=j.tenant_id AND i.sale_id=j.sale_id
		      AND i.status IN ('submitted','authorized')
		  )
	`)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return submitted.RowsAffected() + finalized.RowsAffected() + exhausted.RowsAffected(), nil
}
