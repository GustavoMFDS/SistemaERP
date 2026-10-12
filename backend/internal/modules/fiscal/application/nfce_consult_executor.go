package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/example/sistemaemgo/internal/modules/common"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExecuteNFCeConsultationClaim is the opt-in, consultation-only execution
// boundary. No production or automatic startup caller is registered yet.
// A legitimate authenticated operator (actorUserID) and a valid, leased,
// tenant-scoped claim must both be supplied. This method NEVER calls Authorize.
func (s *FiscalService) ExecuteNFCeConsultationClaim(
	ctx context.Context, pool *pgxpool.Pool, owner, actorUserID string,
	claim NFCeConsultClaim,
) (fisc.NFCeRemoteOutcome, error) {
	if s == nil || s.nfceAuthorizer == nil || s.nfceProcessed == nil ||
		s.uow == nil || s.audit == nil || pool == nil {
		return fisc.NFCeRemoteOutcome{}, common.ErrFiscalNotReady
	}
	if !fiscalLeaseOwnerPattern.MatchString(owner) ||
		claim.Environment != "homologation" ||
		s.validate == nil ||
		s.validate.Var(claim.TenantID, "required,uuid") != nil ||
		s.validate.Var(claim.InvoiceID, "required,uuid") != nil ||
		s.validate.Var(claim.LeaseToken, "required,uuid") != nil ||
		s.validate.Var(actorUserID, "required,uuid") != nil ||
		fisc.ValidateNFCeAccessKey(claim.AccessKey) != nil ||
		!strings.HasPrefix(claim.AccessKey, "31") {
		return fisc.NFCeRemoteOutcome{}, common.ErrValidation
	}

	// Check the current lease, authenticated user membership, source invoice,
	// environment and certificate reference before ANY possible remote call.
	// A remote call may outlive the lease; the final database transaction
	// verifies and locks the same fence again before archiving anything.
	var key, environment, uf, certRef string
	err := pool.QueryRow(ctx, `
		SELECT j.access_key,j.environment,c.address_state,cfg.certificate_secret_ref
		FROM nfce_consult_recovery_jobs j
		JOIN invoices i ON i.tenant_id=j.tenant_id AND i.id=j.invoice_id
		JOIN companies c ON c.id=j.tenant_id
		JOIN nfce_configs cfg ON cfg.tenant_id=j.tenant_id
		JOIN user_tenants ut ON ut.tenant_id=j.tenant_id AND ut.user_id=$6::uuid
		JOIN fiscal_worker_identities w ON w.owner_name=j.lease_owner
		WHERE j.tenant_id=$1::uuid AND j.invoice_id=$2::uuid
		  AND j.lease_token=$3::uuid AND j.lease_owner=$4
		  AND j.state='awaiting_consultation' AND j.lease_until>now()
		  AND j.environment='homologation'
		  AND j.access_key=$5 AND i.access_key=j.access_key
		  AND i.status='submitted' AND i.model=65
		  AND i.environment='homologation'
		  AND upper(btrim(c.address_state))='MG'
		  AND w.revoked_at IS NULL AND w.active_until>now()
		  AND nullif(btrim(cfg.certificate_secret_ref),'') IS NOT NULL
		  AND cfg.environment='homologation'
	`, claim.TenantID, claim.InvoiceID, claim.LeaseToken, owner,
		claim.AccessKey, actorUserID).Scan(&key, &environment, &uf, &certRef)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fisc.NFCeRemoteOutcome{}, ErrNFCeConsultLeaseLost
		}
		return fisc.NFCeRemoteOutcome{}, err
	}

	outcome, err := s.nfceAuthorizer.Consult(ctx, certRef, uf, environment, key)
	if err != nil {
		if finishErr := CompleteNFCeConsultation(ctx, pool, owner, claim,
			NFCeConsultTransportError, 0); finishErr != nil {
			return fisc.NFCeRemoteOutcome{}, errors.Join(err, finishErr)
		}
		return fisc.NFCeRemoteOutcome{}, err
	}
	// The mock or a future adapter cannot certify an authorization by
	// returning just a final-status label. Require all genuine protocol
	// material, matching key and cStat 100 before the transaction.
	if outcome.AccessKey != key ||
		(outcome.StatusCode == 100 && !outcome.Authorized()) ||
		(outcome.Authorized() &&
			(outcome.StatusCode != 100 || strings.TrimSpace(outcome.Protocol) == "" ||
				outcome.ReceivedAt.IsZero() || len(outcome.ProtocolXML) == 0)) {
		if finishErr := CompleteNFCeConsultation(ctx, pool, owner, claim,
			NFCeConsultInvalidProtocol, outcome.StatusCode); finishErr != nil {
			return fisc.NFCeRemoteOutcome{}, finishErr
		}
		return fisc.NFCeRemoteOutcome{}, fmt.Errorf("SEFAZ consultation outcome identity/protocol invalid")
	}
	if outcome.Authorized() {
		result := fisc.NFCeAuthorizationResult{
			Status: fisc.NFCeStatusAuthorized, AccessKey: key,
			Protocol: outcome.Protocol, ProtocolXML: outcome.ProtocolXML,
			AuthorizedAt: outcome.ReceivedAt,
		}
		// Archives protocol, bound nfeProc, status and audit in one transaction
		// with the lease row locked. Lost leases cannot authorize invoices.
		if err := s.applyNFCeAuthorizationResult(ctx, claim.TenantID,
			actorUserID, claim.InvoiceID, result, &nfceConsultFence{
				owner: owner, token: claim.LeaseToken,
			}); err != nil {
			if !errors.Is(err, ErrNFCeConsultLeaseLost) {
				// Treat a failed protocol/archive as unverified. Never
				// return 'authorized' to the operator after a failed tx.
				_ = CompleteNFCeConsultation(ctx, pool, owner, claim,
					NFCeConsultInvalidProtocol, outcome.StatusCode)
			}
			return fisc.NFCeRemoteOutcome{}, err
		}
		return outcome, nil
	}

	// A consultation cannot declare rejection. cStat 217 is *not found at
	// the moment of inquiry*, not permission to issue a replacement.
	// Cancellation/denial/duplicity and inconsistent final labels escalate.
	diagnosis := NFCeConsultPending
	switch {
	case outcome.Rejected(),
		outcome.StatusCode == 101, outcome.StatusCode == 110,
		outcome.StatusCode == 204, outcome.StatusCode == 539:
		diagnosis = NFCeConsultManualReview
	case outcome.StatusCode == 217:
		diagnosis = NFCeConsultNotFound
	case outcome.StatusCode == 0:
		diagnosis = NFCeConsultInvalidProtocol
	}
	if err := CompleteNFCeConsultation(ctx, pool, owner, claim,
		diagnosis, outcome.StatusCode); err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	return outcome, nil
}

// ConsultRecoveryTimeout is a bounded default for explicit invocations of the
// consultation-only executor, not a scheduler or a new SEFAZ retry cadence.
const ConsultRecoveryTimeout = 30 * time.Second
