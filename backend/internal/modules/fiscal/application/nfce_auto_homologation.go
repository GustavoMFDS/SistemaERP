package application

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/modules/common"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	NFCeAutoQueued      = "queued"
	NFCeAutoAwaiting    = "awaiting_sefaz"
	NFCeAutoAuthorized  = "authorized"
	NFCeAutoManual      = "manual_review"
)

var ErrNFCeAutoLeaseLost = errors.New("automatic NFC-e lease expired or replaced")

type NFCeAutoClaim struct {
	TenantID string
	SaleID string
	LeaseToken string
	LeaseUntil time.Time
	Attempts int
}

// PlanNFCeAutomaticHomologation enqueues only new finalized, nonlegacy NFC-e
// sales *after* the store's most recent fiscal configuration change. Backfills
// never auto-transmit old transactions. This function only modifies the queue;
// the server calls it only after explicitly enabled in deployment configuration.
func PlanNFCeAutomaticHomologation(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `
		INSERT INTO nfce_auto_homologation_jobs(tenant_id,sale_id)
		SELECT s.tenant_id,s.id
		FROM sale_fiscal_intents f
		JOIN sales s ON s.tenant_id=f.tenant_id AND s.id=f.sale_id
		JOIN nfce_configs cfg ON cfg.tenant_id=s.tenant_id
		JOIN companies c ON c.id=s.tenant_id
		JOIN user_tenants ut ON ut.tenant_id=s.tenant_id
		  AND ut.user_id=s.created_by_user_id
		LEFT JOIN invoices i ON i.tenant_id=s.tenant_id AND i.sale_id=s.id
		WHERE s.status='finalized' AND s.finalized_at IS NOT NULL
		  AND s.finalized_at>=cfg.updated_at
		  AND f.document_kind='nfce' AND NOT f.legacy_review
		  AND cfg.environment='homologation'
		  AND NULLIF(btrim(cfg.certificate_secret_ref),'') IS NOT NULL
		  AND upper(btrim(c.address_state))='MG'
		  AND (i.id IS NULL OR (i.model=65 AND
		    i.environment='homologation' AND i.status IN ('reserved','signed','submitted')))
		ON CONFLICT (tenant_id,sale_id) DO NOTHING
	`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ClaimNFCeAutomaticHomologation prevents two backend replicas from scheduling
// the same sale simultaneously. Invoice transitions independently enforce
// idempotency, including the signed->submitted boundary BEFORE network IO.
func ClaimNFCeAutomaticHomologation(
	ctx context.Context, pool *pgxpool.Pool, owner string, limit int32, ttl time.Duration,
) ([]NFCeAutoClaim, error) {
	if !fiscalLeaseOwnerPattern.MatchString(owner) || limit<1 || limit>10 ||
		ttl<30*time.Second || ttl>5*time.Minute {
		return nil, fmt.Errorf("invalid automatic NFC-e lease request")
	}
	rows, err := pool.Query(ctx, `
		WITH candidates AS (
		  SELECT j.tenant_id,j.sale_id
		  FROM nfce_auto_homologation_jobs j
		  JOIN sales s ON s.tenant_id=j.tenant_id AND s.id=j.sale_id
		  JOIN sale_fiscal_intents f ON f.tenant_id=j.tenant_id AND f.sale_id=j.sale_id
		  JOIN nfce_configs cfg ON cfg.tenant_id=j.tenant_id
		  JOIN fiscal_worker_identities w ON w.owner_name=$2
		    AND w.revoked_at IS NULL AND w.active_until>now()
		  WHERE j.state='queued' AND j.attempt_count<12
		    AND j.next_attempt_at<=now()
		    AND (j.lease_until IS NULL OR j.lease_until<=now())
		    AND s.status='finalized'
		    AND f.document_kind='nfce' AND NOT f.legacy_review
		    AND cfg.environment='homologation'
		    AND NULLIF(btrim(cfg.certificate_secret_ref),'') IS NOT NULL
		  ORDER BY j.next_attempt_at,j.tenant_id,j.sale_id
		  LIMIT $1 FOR UPDATE OF j SKIP LOCKED
		)
		UPDATE nfce_auto_homologation_jobs j SET
		  lease_token=gen_random_uuid(),lease_owner=$2,
		  lease_until=now()+($3::double precision*interval '1 second'),
		  attempt_count=attempt_count+1,updated_at=now()
		FROM candidates c
		WHERE j.tenant_id=c.tenant_id AND j.sale_id=c.sale_id
		RETURNING j.tenant_id::text,j.sale_id::text,j.lease_token::text,
		          j.lease_until,j.attempt_count
	`, limit, owner, ttl.Seconds())
	if err != nil {return nil,err}
	defer rows.Close()
	out := make([]NFCeAutoClaim,0,limit)
	for rows.Next() {
		var job NFCeAutoClaim
		if err := rows.Scan(&job.TenantID,&job.SaleID,&job.LeaseToken,
			&job.LeaseUntil,&job.Attempts);err!=nil{return nil,err}
		out=append(out,job)
	}
	return out,rows.Err()
}

// CompleteNFCeAutomaticHomologation only updates queue bookkeeping. It cannot
// make an invoice "authorized" and cannot replace a government protocol.
func CompleteNFCeAutomaticHomologation(
	ctx context.Context, pool *pgxpool.Pool, owner string,
	job NFCeAutoClaim, nextState, step string,
) error {
	if !fiscalLeaseOwnerPattern.MatchString(owner) ||
		strings.TrimSpace(job.LeaseToken)=="" {
		return fmt.Errorf("invalid automatic NFC-e completion")
	}
	switch nextState {
	case NFCeAutoQueued,NFCeAutoAwaiting,NFCeAutoAuthorized,NFCeAutoManual:
	default:
		return fmt.Errorf("invalid automatic NFC-e target state")
	}
	switch step {
	case "reserve","sign","authorize","awaiting_sefaz","authorized","manual_review","retry":
	default:
		return fmt.Errorf("invalid automatic NFC-e step")
	}
	tag,err := pool.Exec(ctx,`
		UPDATE nfce_auto_homologation_jobs j SET
		  state=$5,last_step=$6,lease_owner=NULL,lease_token=NULL,lease_until=NULL,
		  next_attempt_at=now()+CASE WHEN $5='queued'
		       THEN interval '5 minutes' ELSE interval '1 day' END,
		  updated_at=now()
		WHERE j.tenant_id=$1 AND j.sale_id=$2
		  AND j.lease_token=$3::uuid AND j.lease_owner=$4
		  AND j.lease_until>clock_timestamp() AND j.state='queued'
		  AND EXISTS (
		    SELECT 1 FROM fiscal_worker_identities w
		    WHERE w.owner_name=$4 AND w.revoked_at IS NULL
		      AND w.active_until>clock_timestamp()
		  )
		  AND EXISTS(
		    SELECT 1 FROM sales s JOIN sale_fiscal_intents f
		      ON f.tenant_id=s.tenant_id AND f.sale_id=s.id
		    WHERE s.tenant_id=j.tenant_id AND s.id=j.sale_id
		      AND s.status='finalized' AND f.document_kind='nfce'
		      AND NOT f.legacy_review
		  )
		  AND ($5<>'authorized' OR EXISTS (
		    SELECT 1 FROM invoices i
		    JOIN invoice_authorized_xml_files x
		      ON x.tenant_id=i.tenant_id AND x.invoice_id=i.id
		    WHERE i.tenant_id=j.tenant_id AND i.sale_id=j.sale_id
		      AND i.status='authorized' AND i.model=65
		      AND i.access_key=x.access_key
		      AND i.authorization_protocol IS NOT NULL
		      AND i.authorized_at IS NOT NULL
		  ))
		  AND ($5<>'awaiting_sefaz' OR EXISTS (
		    SELECT 1 FROM invoices i
		    WHERE i.tenant_id=j.tenant_id AND i.sale_id=j.sale_id
		      AND i.status IN ('submitted','authorized') AND i.model=65
		  ))
	`,job.TenantID,job.SaleID,job.LeaseToken,owner,nextState,step)
	if err!=nil{return err}
	if tag.RowsAffected()!=1{return ErrNFCeAutoLeaseLost}
	return nil
}

// ObserveAuthorizedAutomaticNFCeJob only marks a queue item completed after
// an actual authorized invoice AND the processed signed XML are stored.
func ObserveAuthorizedAutomaticNFCeJob(ctx context.Context, pool *pgxpool.Pool) (int64,error) {
	tag,err:=pool.Exec(ctx,`
		UPDATE nfce_auto_homologation_jobs j SET
		  state='authorized',last_step='authorized',updated_at=now(),
		  lease_owner=NULL,lease_token=NULL,lease_until=NULL
		FROM invoices i JOIN invoice_authorized_xml_files x
		  ON x.tenant_id=i.tenant_id AND x.invoice_id=i.id
		WHERE i.tenant_id=j.tenant_id AND i.sale_id=j.sale_id
		  AND i.model=65 AND i.status='authorized'
		  AND i.access_key=x.access_key
		  AND i.authorization_protocol IS NOT NULL AND i.authorized_at IS NOT NULL
		  AND j.state IN ('queued','awaiting_sefaz')
	`)
	if err!=nil{return 0,err}
	return tag.RowsAffected(),nil
}

// ProcessNFCeAutoHomologationClaim processes one sale by durable stage.
// It never treats a local sale or a signed XML as an authorized fiscal note.
// Submitted means possibly received by SEFAZ: only consultation is allowed.
func ProcessNFCeAutoHomologationClaim(
	ctx context.Context, pool *pgxpool.Pool, svc *FiscalService,
	owner string, job NFCeAutoClaim,
) (string,error) {
	if svc==nil || svc.nfceSigner==nil || svc.nfceValidator==nil ||
		svc.nfceAuthorizer==nil || svc.nfceProcessed==nil {
		return NFCeAutoManual,common.ErrFiscalNotReady
	}
	var actor,invoiceID,status string
	err:=pool.QueryRow(ctx,`
		SELECT s.created_by_user_id::text,
		       COALESCE(i.id::text,''),COALESCE(i.status,'')
		FROM nfce_auto_homologation_jobs j
		JOIN sales s ON s.tenant_id=j.tenant_id AND s.id=j.sale_id
		JOIN user_tenants ut ON ut.user_id=s.created_by_user_id
		  AND ut.tenant_id=s.tenant_id
		JOIN companies c ON c.id=s.tenant_id
		JOIN nfce_configs cfg ON cfg.tenant_id=j.tenant_id
		JOIN sale_fiscal_intents f ON f.tenant_id=j.tenant_id AND f.sale_id=j.sale_id
		JOIN fiscal_worker_identities w ON w.owner_name=j.lease_owner
		LEFT JOIN invoices i ON i.tenant_id=s.tenant_id AND i.sale_id=s.id
		WHERE j.tenant_id=$1 AND j.sale_id=$2
		  AND j.lease_token=$3::uuid AND j.lease_owner=$4
		  AND j.lease_until>clock_timestamp()
		  AND j.state='queued' AND w.revoked_at IS NULL
		  AND w.active_until>clock_timestamp()
		  AND cfg.environment='homologation'
		  AND NULLIF(btrim(cfg.certificate_secret_ref),'') IS NOT NULL
		  AND upper(btrim(c.address_state))='MG'
		  AND s.status='finalized'
		  AND NOT f.legacy_review AND f.document_kind='nfce'
		  AND (i.id IS NULL OR (i.model=65
		    AND i.environment='homologation'))
	`,job.TenantID,job.SaleID,job.LeaseToken,owner).
		Scan(&actor,&invoiceID,&status)
	if errors.Is(err,pgx.ErrNoRows){return NFCeAutoManual,ErrNFCeAutoLeaseLost}
	if err!=nil{return NFCeAutoQueued,err}

	switch status {
	case fisc.NFCeStatusAuthorized:
		return NFCeAutoAuthorized,nil
	case fisc.NFCeStatusSubmitted:
		return NFCeAutoAwaiting,nil
	case fisc.NFCeStatusRejected,fisc.NFCeStatusCancelled:
		return NFCeAutoManual,nil
	case "":
		// Preflight is only a structural blocker, NOT accountant approval.
		preflight,err:=InspectFiscalPreflight(ctx,pool,
			FiscalPreflightClaim{TenantID:job.TenantID,SaleID:job.SaleID})
		if err!=nil{return NFCeAutoQueued,err}
		if preflight!=PreflightInputsUnverified {
			return NFCeAutoQueued,common.ErrFiscalNotReady
		}
		reservation,_,err:=svc.ReserveNFCeDraft(ctx,job.TenantID,actor,
			job.SaleID,time.Now().In(time.FixedZone("BRT",-3*60*60)))
		if err!=nil {
			if errors.Is(err,common.ErrInvoiceAlreadyExists) ||
				errors.Is(err,common.ErrConflict) {
				return NFCeAutoQueued,nil
			}
			return NFCeAutoQueued,err
		}
		invoiceID=reservation.InvoiceID
		status=fisc.NFCeStatusReserved
	case fisc.NFCeStatusReserved,fisc.NFCeStatusSigned:
	default:
		return NFCeAutoManual,common.ErrConflict
	}
	if status==fisc.NFCeStatusReserved {
		if err:=svc.PrepareAutomaticNFCeTaxes(ctx,job.TenantID,actor,invoiceID);err!=nil {
			return NFCeAutoQueued,err
		}
		if _,_,err:=svc.SignNFCeReserved(ctx,job.TenantID,actor,invoiceID);err!=nil {
			if errors.Is(err,common.ErrConflict) {return NFCeAutoQueued,nil}
			return NFCeAutoQueued,err
		}
	}
	// AuthorizeNFCeHomologation persists 'submitted' BEFORE attempting SOAP.
	// A transport timeout leaves it submitted, and this job cannot send again.
	_,err=svc.AuthorizeNFCeHomologation(ctx,job.TenantID,actor,invoiceID)
	if err!=nil {
		var persisted string
		lookupErr:=pool.QueryRow(ctx,`
			SELECT status FROM invoices
			WHERE tenant_id=$1 AND id=$2
		`,job.TenantID,invoiceID).Scan(&persisted)
		if lookupErr==nil && persisted==fisc.NFCeStatusSubmitted {
			return NFCeAutoAwaiting,err
		}
		return NFCeAutoQueued,err
	}
	// Always inspect persisted invoice status instead of inferring completion
	// from the gateway response (which might have been only an ACK).
	var finalStatus string
	if err:=pool.QueryRow(ctx,`
		SELECT status FROM invoices WHERE tenant_id=$1 AND id=$2
	`,job.TenantID,invoiceID).Scan(&finalStatus);err!=nil{
		return NFCeAutoQueued,err
	}
	switch finalStatus {
	case fisc.NFCeStatusAuthorized:return NFCeAutoAuthorized,nil
	case fisc.NFCeStatusSubmitted:return NFCeAutoAwaiting,nil
	case fisc.NFCeStatusRejected:return NFCeAutoManual,nil
	default:return NFCeAutoQueued,nil
	}
}

// StartAutomaticNFCeHomologation is ONLY called when the explicit deployment
// flag, real provider and homologation-only certificate/schema gates are met.
// The worker acts on new sales, not historical purchases or NF-e model 55.
func StartAutomaticNFCeHomologation(
	ctx context.Context, pool *pgxpool.Pool, svc *FiscalService, logger *slog.Logger,
) {
	var random [12]byte
	if _,err:=rand.Read(random[:]);err!=nil {
		logger.Error("nfce_auto_worker_random_failed",slog.Any("error",err))
		return
	}
	owner:=fmt.Sprintf("nfce-auto-hom-%x",random[:])
	regCtx,cancel:=context.WithTimeout(ctx,5*time.Second)
	err:=RegisterFiscalPreflightWorker(regCtx,pool,owner)
	cancel()
	if err!=nil {
		logger.Error("nfce_auto_worker_registration_failed",slog.Any("error",err))
		return
	}
	go func(){
		iteration:=func(){
			tickCtx,cancel:=context.WithTimeout(ctx,90*time.Second)
			defer cancel()
			if err:=RenewFiscalPreflightWorker(tickCtx,pool,owner);err!=nil {
				logger.Error("nfce_auto_worker_identity_expired",slog.Any("error",err))
				return
			}
			_,_ = ObserveAuthorizedAutomaticNFCeJob(tickCtx,pool)
			if _,err:=PlanNFCeAutomaticHomologation(tickCtx,pool);err!=nil {
				if tickCtx.Err()==nil{logger.Warn("nfce_auto_plan_error",slog.Any("error",err))}
				return
			}
			claims,err:=ClaimNFCeAutomaticHomologation(tickCtx,pool,owner,2,3*time.Minute)
			if err!=nil {
				logger.Warn("nfce_auto_claim_error",slog.Any("error",err))
				return
			}
			for _,claim:=range claims{
				if tickCtx.Err()!=nil{return}
				state,err:=ProcessNFCeAutoHomologationClaim(tickCtx,pool,svc,owner,claim)
				step:="retry"
				switch state {
				case NFCeAutoAwaiting:step="awaiting_sefaz"
				case NFCeAutoAuthorized:step="authorized"
				case NFCeAutoManual:step="manual_review"
				}
				if err!=nil {
					logger.Warn("nfce_auto_document_pending",
						slog.String("tenant",claim.TenantID),
						slog.String("sale",claim.SaleID),
						slog.Any("error",err))
				}
				if claim.Attempts>=12 && state==NFCeAutoQueued {
					state=NFCeAutoManual
					step="manual_review"
				}
				if completeErr:=CompleteNFCeAutomaticHomologation(tickCtx,pool,owner,claim,state,step);
					completeErr!=nil && tickCtx.Err()==nil{
					logger.Warn("nfce_auto_completion_error",slog.Any("error",completeErr))
				}
			}
		}
		iteration()
		ticker:=time.NewTicker(10*time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():return
			case <-ticker.C:iteration()
			}
		}
	}()
}
