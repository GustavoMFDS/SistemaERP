package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrFiscalPreparationStale = errors.New("fiscal preparation inputs changed")
var ErrFiscalPreparationLeaseLost = errors.New("fiscal preparation lease no longer valid")

type FiscalEmissionPreparation struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	ResultCode    string    `json:"result_code"`
	PreparedAt    time.Time `json:"prepared_at"`
	SourceMatches bool      `json:"source_matches"`
}

// snapshotQuery deliberately hashes inputs without persisting PII, access
// credentials or raw contents. Every relevant monetary/product/issuer
// field is encoded deterministically by PostgreSQL jsonb, including
// item/payment ordering. This snapshot is NOT a fiscal validation.
const fiscalPreparationSnapshotSQL = `
 SELECT jsonb_build_object(
  'v',1,
  'tenant',s.tenant_id,
  'sale',jsonb_build_object(
    'id',s.id,'status',s.status,'total',s.total,'subtotal',s.subtotal,
    'discount_value',s.discount_value,'customer_id',s.customer_id,
    'finalized_at',s.finalized_at),
  'intent',jsonb_build_object('kind',f.document_kind,'legacy',f.legacy_review),
  'existing_invoice',(
    SELECT jsonb_build_object('id',i.id,'status',i.status,'access_key',i.access_key)
    FROM invoices i WHERE i.tenant_id=s.tenant_id AND i.sale_id=s.id
    ORDER BY i.created_at DESC,i.id DESC LIMIT 1
  ),
  'issuer',(
    SELECT jsonb_build_object(
      'cnpj',c.cnpj,'ie',c.ie,'crt',c.crt,
      'street',c.address_street,'number',c.address_number,
      'complement',c.address_complement,'neighborhood',c.address_neighborhood,
      'city',c.address_city,'municipality',c.address_city_code,
      'state',c.address_state,'zip',c.address_zip)
    FROM companies c WHERE c.id=s.tenant_id
  ),
  'config',(
    SELECT jsonb_build_object('environment',cfg.environment,
      'series',cfg.series,'certificate_reference_present',
      NULLIF(btrim(cfg.certificate_secret_ref),'') IS NOT NULL,
      'certificate_reference_revision',cfg.updated_at,
      'csc_reference_present',NULLIF(btrim(cfg.csc_secret_ref),'') IS NOT NULL)
    FROM nfce_configs cfg WHERE cfg.tenant_id=s.tenant_id
  ),
  'items',COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id',si.id,'product_id',si.product_id,'quantity',si.qty,
      'unit_price',si.unit_price,'discount_value',si.discount_value,
      'subtotal',si.subtotal,'ncm',p.ncm,'cest',p.cest,'unit',p.unit,
      'profile',to_jsonb(fp) - 'updated_by_user_id'
    ) ORDER BY si.id)
    FROM sale_items si
    JOIN products p ON p.id=si.product_id AND p.tenant_id=si.tenant_id
    LEFT JOIN product_fiscal_profiles fp
      ON fp.tenant_id=si.tenant_id AND fp.product_id=si.product_id
    WHERE si.tenant_id=s.tenant_id AND si.sale_id=s.id
  ),'[]'::jsonb),
  'payments',COALESCE((
    SELECT jsonb_agg(jsonb_build_object(
      'id',pay.id,'method',pay.method,'amount',pay.amount
    ) ORDER BY pay.id)
    FROM payments pay WHERE pay.sale_id=s.id
  ),'[]'::jsonb)
 )::text
 FROM sales s
 JOIN sale_fiscal_intents f ON f.tenant_id=s.tenant_id AND f.sale_id=s.id
 WHERE s.tenant_id=$1 AND s.id=$2
`

type preparationReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func fiscalPreparationHash(ctx context.Context, db preparationReader, tenantID, saleID string) (string, error) {
	var body string
	if err := db.QueryRow(ctx, fiscalPreparationSnapshotSQL, tenantID, saleID).Scan(&body); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:]), nil
}

// ObserveFiscalPreparation computes the fingerprint of the data an issuing
// executor would have to re-check before reserving any fiscal number.
// It stores neither certificate refs nor the source document JSON.
func ObserveFiscalPreparation(ctx context.Context, pool *pgxpool.Pool, claim FiscalPreflightClaim, result string) (string, error) {
	if !validFiscalPreflightResult(result) {
		return "", fmt.Errorf("invalid fiscal preparation result")
	}
	return fiscalPreparationHash(ctx, pool, claim.TenantID, claim.SaleID)
}

func validFiscalPreflightResult(result string) bool {
	switch result {
	case PreflightInputsUnverified, PreflightIssuerIncomplete, PreflightProductIncomplete, PreflightPaymentReview, PreflightInternalError:
		return true
	default:
		return false
	}
}

// CompleteFiscalPreflightWithPreparation atomically archives an immutable
// planning observation and releases the current worker lease. A changed
// fingerprint, expired lease, wrong worker or existing invoice fails closed;
// no plan is inserted in those cases. This function NEVER issues invoices.
func CompleteFiscalPreflightWithPreparation(
	ctx context.Context, pool *pgxpool.Pool, owner string,
	claim FiscalPreflightClaim, result, expectedHash string,
) error {
	if !fiscalLeaseOwnerPattern.MatchString(owner) || !validFiscalPreflightResult(result) ||
		len(expectedHash) != 64 {
		return fmt.Errorf("invalid fiscal preparation arguments")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var identity string
	err = tx.QueryRow(ctx, `
  SELECT w.id::text
  FROM fiscal_dispatch_jobs j
  JOIN sale_fiscal_intents f ON f.tenant_id=j.tenant_id AND f.sale_id=j.sale_id
  JOIN sales s ON s.tenant_id=j.tenant_id AND s.id=j.sale_id
  JOIN nfce_configs cfg ON cfg.tenant_id=j.tenant_id
  JOIN fiscal_worker_identities w ON w.owner_name=j.lease_owner
  WHERE j.tenant_id=$1 AND j.sale_id=$2
    AND j.lease_token=$3::uuid AND j.lease_owner=$4
    AND j.lease_until>now() AND j.state='awaiting_fiscal_processing'
    AND w.revoked_at IS NULL AND w.active_until>now()
    AND s.status='finalized' AND NOT f.legacy_review
    AND f.document_kind='nfce' AND cfg.environment='homologation'
    AND NOT EXISTS (
     SELECT 1 FROM invoices i WHERE i.tenant_id=j.tenant_id AND i.sale_id=j.sale_id
    )
  FOR UPDATE OF j
 `, claim.TenantID, claim.SaleID, claim.LeaseToken, owner).Scan(&identity)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrFiscalPreparationLeaseLost
	}
	if err != nil {
		return err
	}
	currentHash, err := fiscalPreparationHash(ctx, tx, claim.TenantID, claim.SaleID)
	if err != nil {
		return err
	}
	if currentHash != expectedHash {
		return ErrFiscalPreparationStale
	}
	// The persisted result must also be recalculated while the same
	// lease row is locked, otherwise a caller could claim that
	// incomplete issuer/product/payment fields had passed inspection.
	verified, verifyErr := inspectFiscalPreflight(ctx, tx, claim)
	if verifyErr != nil || verified != result {
		return ErrFiscalPreparationStale
	}
	status := "blocked"
	if result == PreflightInputsUnverified {
		status = "tax_review_required"
	}
	_, err = tx.Exec(ctx, `
  INSERT INTO fiscal_emission_preparations(
   tenant_id,sale_id,worker_identity_id,lease_token,
   source_sha256,status,result_code
  ) VALUES($1,$2,$3::uuid,$4::uuid,$5,$6,$7)
 `, claim.TenantID, claim.SaleID, identity, claim.LeaseToken, currentHash, status, result)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
  UPDATE fiscal_dispatch_jobs SET
    lease_token=NULL,lease_owner=NULL,lease_until=NULL,
    last_result_code=$5,
    next_claim_at=now()+CASE
      WHEN $5='internal_error' THEN interval '5 minutes'
      WHEN $5='inputs_present_unverified' THEN interval '1 hour'
      ELSE interval '3 hours'
    END
  WHERE tenant_id=$1 AND sale_id=$2 AND lease_token=$3::uuid
    AND lease_owner=$4 AND lease_until>now() AND state='awaiting_fiscal_processing'
 `, claim.TenantID, claim.SaleID, claim.LeaseToken, owner, result)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrFiscalPreparationLeaseLost
	}
	return tx.Commit(ctx)
}

// GetLatestFiscalPreparation returns the last archived diagnostic and whether
// its SHA-256 STILL matches live sale, tax profile and payment inputs.
// No record may ever be interpreted as an SEFAZ approval.
func GetLatestFiscalPreparation(
	ctx context.Context, pool *pgxpool.Pool, tenantID, saleID string,
) (FiscalEmissionPreparation, error) {
	var out FiscalEmissionPreparation
	var hash string
	err := pool.QueryRow(ctx, `
  SELECT id::text,status,result_code,prepared_at,source_sha256
  FROM fiscal_emission_preparations
  WHERE tenant_id=$1 AND sale_id=$2
  ORDER BY prepared_at DESC,id DESC LIMIT 1
 `, tenantID, saleID).Scan(&out.ID, &out.Status, &out.ResultCode, &out.PreparedAt, &hash)
	if err != nil {
		return out, err
	}
	currentHash, err := fiscalPreparationHash(ctx, pool, tenantID, saleID)
	if err != nil {
		return out, err
	}
	out.SourceMatches = currentHash == hash
	return out, nil
}
