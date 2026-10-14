package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNFCeConsultLeaseLost = errors.New("NFC-e consultation lease lost or expired")

// NFCeConsultClaim represents the exclusive permission to CONSULT one
// already-submitted access key. It never permits reauthorization or re-sending.
type NFCeConsultClaim struct {
	TenantID     string
	InvoiceID    string
	AccessKey    string
	Environment  string
	LeaseToken   string
	LeaseUntil   time.Time
	AttemptCount int
}

const (
	NFCeConsultPending         = "pending"
	NFCeConsultNotFound        = "not_found"
	NFCeConsultTransportError  = "transport_error"
	NFCeConsultInvalidProtocol = "invalid_protocol"
	NFCeConsultManualReview    = "manual_review"
)

// PlanNFCeSubmittedConsultations records, idempotently, invoices whose
// authorization result is uncertain. Only homogeneous MG/homologation
// submissions are eligible. This function performs SQL only; no SEFAZ IO.
func PlanNFCeSubmittedConsultations(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `
		INSERT INTO nfce_consult_recovery_jobs(tenant_id,invoice_id,access_key,environment)
		SELECT i.tenant_id,i.id,i.access_key,i.environment
		FROM invoices i
		JOIN companies c ON c.id=i.tenant_id
		WHERE i.model=65 AND i.status='submitted'
		  AND i.environment='homologation'
		  AND upper(btrim(c.address_state))='MG'
		  AND i.access_key IS NOT NULL
		  AND i.document_number IS NOT NULL
		ON CONFLICT (tenant_id,invoice_id) DO NOTHING
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ResolveNFCeConsultationPlans only observes already-final invoice rows.
// It never decides what SEFAZ said or updates an invoice itself.
func ResolveNFCeConsultationPlans(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE nfce_consult_recovery_jobs j SET
		  state='resolved',lease_token=NULL,lease_owner=NULL,lease_until=NULL,
		  updated_at=now()
		FROM invoices i
		WHERE i.tenant_id=j.tenant_id AND i.id=j.invoice_id
		  AND i.status IN ('authorized','rejected','cancelled')
		  AND j.state<>'resolved'
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ClaimNFCeSubmittedConsultations fences concurrent workers with an
// expiring identity, database row lock and a single-use lease token.
// This operation only returns identities and does not contact SEFAZ.
func ClaimNFCeSubmittedConsultations(
	ctx context.Context, pool *pgxpool.Pool, owner string, max int32, ttl time.Duration,
) ([]NFCeConsultClaim, error) {
	if !fiscalLeaseOwnerPattern.MatchString(owner) || max < 1 || max > 25 ||
		ttl < 10*time.Second || ttl > 5*time.Minute {
		return nil, fmt.Errorf("invalid consultation claim parameters")
	}
	rows, err := pool.Query(ctx, `
		WITH candidates AS (
		  SELECT j.tenant_id,j.invoice_id
		  FROM nfce_consult_recovery_jobs j
		  JOIN invoices i ON i.tenant_id=j.tenant_id AND i.id=j.invoice_id
		  JOIN fiscal_worker_identities worker
		    ON worker.owner_name=$2 AND worker.revoked_at IS NULL
		       AND worker.active_until>now()
		  WHERE j.state='awaiting_consultation'
		    AND j.attempt_count<10
		    AND j.next_attempt_at<=now()
		    AND (j.lease_until IS NULL OR j.lease_until<=now())
		    AND i.status='submitted' AND i.model=65
		    AND i.environment='homologation' AND i.access_key=j.access_key
		  ORDER BY j.next_attempt_at,j.tenant_id,j.invoice_id
		  LIMIT $1 FOR UPDATE OF j SKIP LOCKED
		)
		UPDATE nfce_consult_recovery_jobs j SET
		  lease_token=gen_random_uuid(),lease_owner=$2,
		  lease_until=now()+($3::double precision * interval '1 second'),
		  attempt_count=attempt_count+1,updated_at=now()
		FROM candidates c
		WHERE j.tenant_id=c.tenant_id AND j.invoice_id=c.invoice_id
		RETURNING j.tenant_id::text,j.invoice_id::text,j.access_key,j.environment,
		  j.lease_token::text,j.lease_until,j.attempt_count
	`, max, owner, ttl.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NFCeConsultClaim, 0, max)
	for rows.Next() {
		var item NFCeConsultClaim
		if err := rows.Scan(&item.TenantID, &item.InvoiceID, &item.AccessKey,
			&item.Environment, &item.LeaseToken, &item.LeaseUntil, &item.AttemptCount); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// CompleteNFCeConsultation records a non-authoritative diagnosis and the next
// safe consultation window. It never marks an invoice authorized or rejected.
// Inconclusive/failed consultations cannot trigger a blind authorization retry.
func CompleteNFCeConsultation(
	ctx context.Context, pool *pgxpool.Pool, owner string, claim NFCeConsultClaim,
	result string, statusCode int,
) error {
	if !fiscalLeaseOwnerPattern.MatchString(owner) ||
		strings.TrimSpace(claim.LeaseToken) == "" ||
		strings.TrimSpace(claim.TenantID) == "" ||
		strings.TrimSpace(claim.InvoiceID) == "" {
		return fmt.Errorf("invalid consultation completion identity")
	}
	switch result {
	case NFCeConsultPending, NFCeConsultNotFound, NFCeConsultTransportError,
		NFCeConsultInvalidProtocol, NFCeConsultManualReview:
	default:
		return fmt.Errorf("invalid consultation result")
	}
	if statusCode < 0 || statusCode > 999 {
		return fmt.Errorf("invalid SEFAZ status code")
	}
	tag, err := pool.Exec(ctx, `
		UPDATE nfce_consult_recovery_jobs j SET
		  lease_token=NULL,lease_owner=NULL,lease_until=NULL,
		  last_result_code=$5,last_status_code=NULLIF($6,0),
		  state=CASE WHEN $5='manual_review' OR j.attempt_count>=10
		       THEN 'manual_review' ELSE 'awaiting_consultation' END,
		  next_attempt_at=now()+CASE
		    WHEN $5='not_found' THEN interval '20 minutes'
		    WHEN $5='pending' THEN interval '5 minutes'
		    WHEN $5='transport_error' THEN interval '15 minutes'
		    ELSE interval '1 hour' END,
		  updated_at=now()
		WHERE j.tenant_id=$1 AND j.invoice_id=$2 AND j.lease_token=$3::uuid
		  AND j.lease_owner=$4 AND j.lease_until>now()
		  AND j.state='awaiting_consultation'
		  AND EXISTS (
		    SELECT 1 FROM fiscal_worker_identities w
		    WHERE w.owner_name=$4 AND w.revoked_at IS NULL AND w.active_until>now()
		  )
		  AND EXISTS (
		    SELECT 1 FROM invoices i
		    WHERE i.tenant_id=j.tenant_id AND i.id=j.invoice_id
		      AND i.status='submitted' AND i.model=65
		      AND i.environment='homologation' AND i.access_key=j.access_key
		  )
	`, claim.TenantID, claim.InvoiceID, claim.LeaseToken, owner, result, statusCode)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNFCeConsultLeaseLost
	}
	return nil
}

// ReconcileExhaustedNFCeConsultations catches a crash after the final
// leased CONSULT attempt. No invoice status or SEFAZ record is altered;
// the work is explicitly escalated for human investigation rather than
// remaining indefinitely invisible in an unclaimable due queue.
func ReconcileExhaustedNFCeConsultations(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE nfce_consult_recovery_jobs j SET
		  state='manual_review',last_result_code='manual_review',
		  lease_token=NULL,lease_owner=NULL,lease_until=NULL,updated_at=now()
		FROM invoices i
		WHERE i.tenant_id=j.tenant_id AND i.id=j.invoice_id
		  AND i.status='submitted' AND i.model=65
		  AND i.environment='homologation' AND i.access_key=j.access_key
		  AND j.state='awaiting_consultation' AND j.attempt_count>=10
		  AND (j.lease_until IS NULL OR j.lease_until<=now())
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// StartNFCeConsultationPlanner tracks unresolved submissions automatically
// without network calls, invoice-number reservation or any tax operation.
// A SEFAZ consultation executor must be separately enabled only after
// genuine credentials/homologation; none is started here.
func StartNFCeConsultationPlanner(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	go func() {
		plan := func() {
			scanCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			if _, err := ResolveNFCeConsultationPlans(scanCtx, pool); err != nil && scanCtx.Err() == nil {
				logger.Warn("nfce_consult_resolve_failed", slog.Any("err", err))
				return
			}
			if _, err := ReconcileExhaustedNFCeConsultations(scanCtx, pool); err != nil {
				if scanCtx.Err() == nil {
					logger.Warn("nfce_consult_exhausted_failed", slog.Any("err", err))
				}
				return
			}
			if _, err := PlanNFCeSubmittedConsultations(scanCtx, pool); err != nil && scanCtx.Err() == nil {
				logger.Warn("nfce_consult_plan_failed", slog.Any("err", err))
			}
		}
		plan()
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				plan()
			}
		}
	}()
}
