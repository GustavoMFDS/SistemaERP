package application

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StartAutomaticNFCeConsultation starts the separately gated, consult-only
// reconciliation worker. It never invokes the Authorize method and never
// treats a not-found response as safe to re-send. It uses the same validated
// tenant/operator, durable lease, and nfeProc archival as the internal
// consultation executor. Production isn't enabled by this worker.
func StartAutomaticNFCeConsultation(
	ctx context.Context, pool *pgxpool.Pool, svc *FiscalService, logger *slog.Logger,
) {
	var random [12]byte
	if _,err:=rand.Read(random[:]);err!=nil {
		logger.Error("nfce_consult_worker_random_failed",slog.Any("error",err))
		return
	}
	owner:=fmt.Sprintf("nfce-auto-consult-%x",random[:])
	regCtx,cancel:=context.WithTimeout(ctx,5*time.Second)
	err:=RegisterFiscalPreflightWorker(regCtx,pool,owner)
	cancel()
	if err!=nil{
		logger.Error("nfce_consult_worker_registration_failed",slog.Any("error",err))
		return
	}
	go func(){
		run:=func(){
			tickCtx,cancel:=context.WithTimeout(ctx,90*time.Second)
			defer cancel()
			if err:=RenewFiscalPreflightWorker(tickCtx,pool,owner);err!=nil{
				logger.Error("nfce_consult_identity_expired",slog.Any("error",err))
				return
			}
			_,_ = ResolveNFCeConsultationPlans(tickCtx,pool)
			if _,err:=PlanNFCeSubmittedConsultations(tickCtx,pool);err!=nil{
				if tickCtx.Err()==nil{logger.Warn("nfce_consult_plan_failed",slog.Any("error",err))}
				return
			}
			claims,err:=ClaimNFCeSubmittedConsultations(tickCtx,pool,owner,2,3*time.Minute)
			if err!=nil{
				logger.Warn("nfce_consult_claim_failed",slog.Any("error",err))
				return
			}
			for _,claim:=range claims{
				if tickCtx.Err()!=nil{return}
				var actor string
				err:=pool.QueryRow(tickCtx,`
					SELECT s.created_by_user_id::text
					FROM invoices i
					JOIN sales s ON s.tenant_id=i.tenant_id AND s.id=i.sale_id
					JOIN user_tenants ut
					  ON ut.user_id=s.created_by_user_id AND ut.tenant_id=i.tenant_id
					WHERE i.tenant_id=$1 AND i.id=$2 AND i.status='submitted'
					  AND i.environment='homologation' AND i.access_key=$3
				`,claim.TenantID,claim.InvoiceID,claim.AccessKey).Scan(&actor)
				if errors.Is(err,pgx.ErrNoRows){
					_ = CompleteNFCeConsultation(tickCtx,pool,owner,claim,
						NFCeConsultManualReview,0)
					continue
				}
				if err!=nil{
					logger.Warn("nfce_consult_actor_lookup_failed",slog.Any("error",err))
					continue
				}
				if _,err:=svc.ExecuteNFCeConsultationClaim(tickCtx,pool,owner,actor,claim);
					err!=nil {
					logger.Warn("nfce_consult_document_pending",
						slog.String("tenant",claim.TenantID),
						slog.String("invoice",claim.InvoiceID),
						slog.Any("error",err))
				}
			}
		}
		run()
		ticker:=time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():return
			case <-ticker.C:run()
			}
		}
	}()
}
