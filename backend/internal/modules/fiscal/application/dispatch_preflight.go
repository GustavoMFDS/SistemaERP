package application

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrFiscalLeaseLost      = errors.New("fiscal preflight lease lost or expired")
	fiscalLeaseOwnerPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,63}$`)
)

type FiscalPreflightClaim struct {
	TenantID   string
	SaleID     string
	LeaseToken string
	LeaseUntil time.Time
}

const (
	PreflightInputsUnverified  = "inputs_present_unverified"
	PreflightIssuerIncomplete  = "issuer_incomplete"
	PreflightProductIncomplete = "product_classification_missing"
	PreflightPaymentReview     = "payment_review_required"
	PreflightInternalError     = "internal_error"
)

// RegisterFiscalPreflightWorker registers an ephemeral machine identity.
// This principal is NOT a user account and is never used to impersonate staff.
// A random, process-scoped owner can be registered exactly once.
func RegisterFiscalPreflightWorker(ctx context.Context, pool *pgxpool.Pool, owner string) error {
	if !fiscalLeaseOwnerPattern.MatchString(owner) {
		return fmt.Errorf("invalid fiscal worker identity")
	}
	var id string
	err := pool.QueryRow(ctx, `
  INSERT INTO fiscal_worker_identities(owner_name,active_until)
  VALUES($1,now()+interval '5 minutes') RETURNING id::text
 `, owner).Scan(&id)
	return err
}

// RenewFiscalPreflightWorker keeps an already active technical identity
// alive. An expired or revoked principal cannot silently re-register.
func RenewFiscalPreflightWorker(ctx context.Context, pool *pgxpool.Pool, owner string) error {
	if !fiscalLeaseOwnerPattern.MatchString(owner) {
		return fmt.Errorf("invalid fiscal worker identity")
	}
	tag, err := pool.Exec(ctx, `
  UPDATE fiscal_worker_identities
  SET renewed_at=now(),active_until=now()+interval '5 minutes'
  WHERE owner_name=$1 AND revoked_at IS NULL AND active_until>now()
 `, owner)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("fiscal worker identity expired or revoked")
	}
	return nil
}

// ClaimFiscalPreflight atomically fences duplicate *LOCAL* inspections across
// replicas. It cannot reserve fiscal numbers or send anything to SEFAZ.
// Only finalized, nonlegacy NFC-e obligations in homologation with a configured
// certificate *reference* (not a validated certificate) are claimable.
func ClaimFiscalPreflight(ctx context.Context, pool *pgxpool.Pool, owner string, max int32, ttl time.Duration) ([]FiscalPreflightClaim, error) {
	if !fiscalLeaseOwnerPattern.MatchString(owner) || max < 1 || max > 25 || ttl < 10*time.Second || ttl > 5*time.Minute {
		return nil, fmt.Errorf("invalid fiscal preflight lease parameters")
	}
	rows, err := pool.Query(ctx, `
  WITH candidates AS (
   SELECT j.tenant_id,j.sale_id
   FROM fiscal_dispatch_jobs j
   JOIN sale_fiscal_intents f ON f.tenant_id=j.tenant_id AND f.sale_id=j.sale_id
   JOIN sales s ON s.tenant_id=j.tenant_id AND s.id=j.sale_id
   JOIN nfce_configs cfg ON cfg.tenant_id=j.tenant_id
   JOIN fiscal_worker_identities worker
     ON worker.owner_name=$2 AND worker.revoked_at IS NULL
     AND worker.active_until>now()
   WHERE j.state='awaiting_fiscal_processing'
     AND j.next_claim_at<=now()
     AND (j.lease_until IS NULL OR j.lease_until<=now())
     AND NOT f.legacy_review AND f.document_kind='nfce'
     AND s.status='finalized'
     AND cfg.environment='homologation'
     AND NULLIF(btrim(cfg.certificate_secret_ref),'') IS NOT NULL
     AND NOT EXISTS (
       SELECT 1 FROM invoices i
       WHERE i.tenant_id=j.tenant_id AND i.sale_id=j.sale_id
     )
   ORDER BY j.next_claim_at,j.tenant_id,j.sale_id
   LIMIT $1
   FOR UPDATE OF j SKIP LOCKED
  )
  UPDATE fiscal_dispatch_jobs j SET
   lease_token=gen_random_uuid(),lease_owner=$2,
   lease_until=now()+($3::double precision * interval '1 second'),
   claim_count=claim_count+1,last_claimed_at=now()
  FROM candidates c
  WHERE j.tenant_id=c.tenant_id AND j.sale_id=c.sale_id
  RETURNING j.tenant_id::text,j.sale_id::text,j.lease_token::text,j.lease_until
 `, max, owner, ttl.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	claims := make([]FiscalPreflightClaim, 0, max)
	for rows.Next() {
		var claim FiscalPreflightClaim
		if err := rows.Scan(&claim.TenantID, &claim.SaleID, &claim.LeaseToken, &claim.LeaseUntil); err != nil {
			return nil, err
		}
		claims = append(claims, claim)
	}
	return claims, rows.Err()
}

// CompleteFiscalPreflight releases the lease ONLY for the matching owner and
// token while still live. An old process cannot overwrite the new owner's
// result after a crash/lease expiry. Result labels are diagnostic only.
func CompleteFiscalPreflight(ctx context.Context, pool *pgxpool.Pool, owner string, claim FiscalPreflightClaim, result string) error {
	switch result {
	case PreflightInputsUnverified, PreflightIssuerIncomplete, PreflightProductIncomplete, PreflightPaymentReview, PreflightInternalError:
	default:
		return fmt.Errorf("invalid fiscal preflight result")
	}
	if !fiscalLeaseOwnerPattern.MatchString(owner) {
		return fmt.Errorf("invalid lease owner")
	}
	tag, err := pool.Exec(ctx, `
  UPDATE fiscal_dispatch_jobs SET
   lease_token=NULL,lease_owner=NULL,lease_until=NULL,
   last_result_code=$5,
   next_claim_at=now()+CASE
    WHEN $5='internal_error' THEN interval '5 minutes'
    WHEN $5='inputs_present_unverified' THEN interval '1 hour'
    ELSE interval '3 hours'
   END
  WHERE tenant_id=$1 AND sale_id=$2 AND lease_token=$3::uuid
    AND lease_owner=$4 AND lease_until>now()
    AND state='awaiting_fiscal_processing'
    AND EXISTS (
      SELECT 1 FROM fiscal_worker_identities w
      WHERE w.owner_name=$4 AND w.revoked_at IS NULL AND w.active_until>now()
    )
    AND NOT EXISTS (
      SELECT 1 FROM invoices i WHERE i.tenant_id=$1 AND i.sale_id=$2
    )
 `, claim.TenantID, claim.SaleID, claim.LeaseToken, owner, result)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrFiscalLeaseLost
	}
	return nil
}

// InspectFiscalPreflight only checks a few mandatory fields and unsupported
// payment methods. Even "inputs_present_unverified" NEVER means the tax model,
// signature, SEFAZ environment, or the invoice is ready for issuance.
func InspectFiscalPreflight(ctx context.Context, pool *pgxpool.Pool, claim FiscalPreflightClaim) (string, error) {
	return inspectFiscalPreflight(ctx, pool, claim)
}

func inspectFiscalPreflight(ctx context.Context, reader preparationReader, claim FiscalPreflightClaim) (string, error) {
	var profile fisc.NFCeIssuerProfile
	err := reader.QueryRow(ctx, `
  SELECT c.legal_name,c.cnpj,COALESCE(c.ie,''),COALESCE(c.crt,''),
    COALESCE(c.address_street,''),COALESCE(c.address_number,''),
    COALESCE(c.address_neighborhood,''),COALESCE(c.address_city,''),
    COALESCE(c.address_city_code,''),COALESCE(c.address_state,''),
    COALESCE(c.address_zip,'')
  FROM companies c
  JOIN sales s ON s.tenant_id=c.id AND s.id=$2
  WHERE c.id=$1 AND s.status='finalized'
 `, claim.TenantID, claim.SaleID).Scan(
		&profile.LegalName, &profile.CNPJ, &profile.IE, &profile.CRT,
		&profile.AddressStreet, &profile.AddressNumber,
		&profile.AddressNeighborhood, &profile.AddressCity,
		&profile.AddressCityCode, &profile.AddressState, &profile.AddressZIP,
	)
	if err != nil {
		return PreflightInternalError, err
	}
	// Share the structural checks with readiness/production gates. A local
	// inspection must never downgrade a missing issuer field to "unverified".
	if len(fisc.NFCeMGIssuerLocalBlockers(profile)) != 0 {
		return PreflightIssuerIncomplete, nil
	}
	var totalItems, badProducts, totalPayments, badPayments int
	err = reader.QueryRow(ctx, `
 SELECT
  (SELECT count(*) FROM sale_items si WHERE si.tenant_id=$1 AND si.sale_id=$2),
  (SELECT count(*) FROM sale_items si
    JOIN products p ON p.tenant_id=si.tenant_id AND p.id=si.product_id
    LEFT JOIN product_fiscal_profiles fp ON fp.tenant_id=si.tenant_id AND fp.product_id=si.product_id
    WHERE si.tenant_id=$1 AND si.sale_id=$2
      AND (p.ncm IS NULL OR fp.product_id IS NULL)),
  (SELECT count(*) FROM payments pay JOIN sales s ON s.id=pay.sale_id
    WHERE s.tenant_id=$1 AND s.id=$2),
  (SELECT count(*) FROM payments pay JOIN sales s ON s.id=pay.sale_id
    WHERE s.tenant_id=$1 AND s.id=$2 AND pay.method NOT IN ('cash','pix','transfer'))
 `, claim.TenantID, claim.SaleID).Scan(&totalItems, &badProducts, &totalPayments, &badPayments)
	if err != nil {
		return PreflightInternalError, err
	}
	if totalItems == 0 || badProducts != 0 {
		return PreflightProductIncomplete, nil
	}
	if totalPayments == 0 || badPayments != 0 {
		return PreflightPaymentReview, nil
	}
	return PreflightInputsUnverified, nil
}

// StartFiscalPreflightDiagnostics is an opt-in-free, local-only safety pass.
// It can run concurrently on multiple instances but cannot issue invoices.
// All remote actions remain explicitly forbidden in this worker.
func StartFiscalPreflightDiagnostics(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		logger.Error("fiscal_preflight_owner_id_unavailable", slog.Any("error", err))
		return
	}
	owner := fmt.Sprintf("local-preflight-%x", random[:])
	regCtx, regCancel := context.WithTimeout(ctx, 5*time.Second)
	registerErr := RegisterFiscalPreflightWorker(regCtx, pool, owner)
	regCancel()
	if registerErr != nil {
		logger.Error("fiscal_preflight_identity_registration_failed", slog.Any("error", registerErr))
		return
	}
	go func() {
		run := func() {
			claimCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := RenewFiscalPreflightWorker(claimCtx, pool, owner); err != nil {
				if claimCtx.Err() == nil {
					logger.Error("fiscal_preflight_identity_renewal_failed", slog.Any("error", err))
				}
				return
			}
			claims, err := ClaimFiscalPreflight(claimCtx, pool, owner, 10, 2*time.Minute)
			if err != nil {
				if claimCtx.Err() == nil {
					logger.Warn("fiscal_preflight_claim_failed", slog.Any("err", err))
				}
				return
			}
			for _, claim := range claims {
				if claimCtx.Err() != nil {
					return
				}
				result, err := InspectFiscalPreflight(claimCtx, pool, claim)
				if err != nil {
					logger.Warn("fiscal_preflight_inspection_failed", slog.Any("err", err))
					result = PreflightInternalError
				}
				// Store a versioned fingerprint together with the completion
				// of the machine lease. A stale or lost claim must NEVER
				// leave behind an apparently current preparation plan.
				hash, snapshotErr := ObserveFiscalPreparation(claimCtx, pool, claim, result)
				if snapshotErr != nil {
					logger.Warn("fiscal_preparation_snapshot_failed", slog.Any("err", snapshotErr))
					// No plan is archived if inputs could not be read.
					if releaseErr := CompleteFiscalPreflight(claimCtx, pool, owner, claim, PreflightInternalError); releaseErr != nil {
						logger.Warn("fiscal_preflight_release_failed", slog.Any("err", releaseErr))
					}
					continue
				}
				if err := CompleteFiscalPreflightWithPreparation(claimCtx, pool, owner, claim, result, hash); err != nil {
					logger.Warn("fiscal_preflight_preparation_failed", slog.Any("err", err))
				}
			}
		}
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
