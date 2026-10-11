//go:build integration

package integration_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	fiscalapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Fiscal dispatch never issues documents: this test only verifies durable
// obligation and automatic status reconciliation in disposable PostgreSQL.
func TestFiscalDispatchClassifiesWithoutIssuing(t *testing.T) {
	if os.Getenv("TEST_FISCAL_DISPOSABLE_DB") != "1" {
		t.Skip("disposable fiscal database required")
	}
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var tenant, actor string
	if err := pool.QueryRow(ctx, `
 SELECT utr.tenant_id::text,utr.user_id::text
 FROM user_tenant_roles utr
 JOIN roles r ON r.id=utr.role_id AND r.name='admin'
 ORDER BY utr.user_id LIMIT 1
 `).Scan(&tenant, &actor); err != nil {
		t.Fatal(err)
	}
	var register, session, sale string
	if err := pool.QueryRow(ctx, `
 INSERT INTO cash_registers(tenant_id,name,active)
 VALUES($1,'Fiscal Dispatch Test',true) RETURNING id::text
 `, tenant).Scan(&register); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
 INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,opening_amount,status)
 VALUES($1,$2,$3,0,'open') RETURNING id::text
 `, tenant, register, actor).Scan(&session); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
 INSERT INTO sales(tenant_id,cash_session_id,status,subtotal,discount_value,total,
 profit_estimated,created_by_user_id)
 VALUES($1,$2,'finalized',10,0,10,5,$3) RETURNING id::text
 `, tenant, session, actor).Scan(&sale); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := pool.QueryRow(ctx, `
 SELECT state FROM fiscal_dispatch_jobs WHERE tenant_id=$1 AND sale_id=$2
 `, tenant, sale).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "pending" {
		t.Fatalf("sale was not enqueued: %s", state)
	}
	if _, err := fiscalapp.ReconcileFiscalDispatchBatch(ctx, pool, 100); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
 SELECT state FROM fiscal_dispatch_jobs WHERE tenant_id=$1 AND sale_id=$2
 `, tenant, sale).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "awaiting_configuration" {
		t.Fatalf("unexpected status without certificate: %s", state)
	}
	var invoices int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM invoices WHERE tenant_id=$1 AND sale_id=$2", tenant, sale).Scan(&invoices); err != nil {
		t.Fatal(err)
	}
	if invoices != 0 {
		t.Fatal("status reconciliation created invoice")
	}
	// Issuer configuration is a reference only; local classification MUST NOT
	// sign, reserve, authorize or transmit even when that reference exists.
	if _, err := pool.Exec(ctx, `
 INSERT INTO nfce_configs(tenant_id,certificate_secret_ref)
 VALUES($1,'secret://fiscal-dispatch-test')
 ON CONFLICT(tenant_id) DO UPDATE SET certificate_secret_ref=EXCLUDED.certificate_secret_ref
 `, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
 UPDATE fiscal_dispatch_jobs SET next_check_at=now()-interval '1 second'
 WHERE tenant_id=$1 AND sale_id=$2
 `, tenant, sale); err != nil {
		t.Fatal(err)
	}
	if _, err := fiscalapp.ReconcileFiscalDispatchBatch(ctx, pool, 100); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
 SELECT state FROM fiscal_dispatch_jobs WHERE tenant_id=$1 AND sale_id=$2
 `, tenant, sale).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "awaiting_fiscal_processing" {
		t.Fatalf("incorrect readiness classification: %s", state)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM invoices WHERE tenant_id=$1 AND sale_id=$2", tenant, sale).Scan(&invoices); err != nil {
		t.Fatal(err)
	}
	if invoices != 0 {
		t.Fatal("diagnostic worker illegally reserved a fiscal invoice")
	}
	// An unregistered process must not be able to claim fiscal work.
	anonymousClaims, err := fiscalapp.ClaimFiscalPreflight(ctx, pool, "unregistered-worker", 2, time.Minute)
	if err != nil || len(anonymousClaims) != 0 {
		t.Fatalf("unregistered worker claimed jobs: count=%d err=%v", len(anonymousClaims), err)
	}
	// All workers must first register unique machine identities, distinct
	// from employee user IDs. Claims without registration fail closed.
	for _, worker := range []string{"replica-a", "replica-b", "worker-one", "worker-two"} {
		if err := fiscalapp.RegisterFiscalPreflightWorker(ctx, pool, worker); err != nil {
			t.Fatalf("cannot register technical worker %s: %v", worker, err)
		}
	}
	// Two replicas must not own the same fiscal preflight at once.
	claimsA, err := fiscalapp.ClaimFiscalPreflight(ctx, pool, "replica-a", 2, time.Minute)
	if err != nil || len(claimsA) != 1 {
		t.Fatalf("first lease: count=%d err=%v", len(claimsA), err)
	}
	claimsB, err := fiscalapp.ClaimFiscalPreflight(ctx, pool, "replica-b", 2, time.Minute)
	if err != nil || len(claimsB) != 0 {
		t.Fatalf("duplicate lease granted: count=%d err=%v", len(claimsB), err)
	}
	result, err := fiscalapp.InspectFiscalPreflight(ctx, pool, claimsA[0])
	if err != nil || result != fiscalapp.PreflightIssuerIncomplete {
		t.Fatalf("preflight did not refuse incomplete issuer: %s %v", result, err)
	}
	if err := fiscalapp.CompleteFiscalPreflight(ctx, pool, "replica-b", claimsA[0], result); !errors.Is(err, fiscalapp.ErrFiscalLeaseLost) {
		t.Fatalf("wrong owner able to release another lease: %v", err)
	}
	// Simulate the first process crashing. The next owner is permitted
	// to reclaim expired work, but the stale token cannot complete it.
	if _, err := pool.Exec(ctx, `
	  UPDATE fiscal_dispatch_jobs SET lease_until=now()-interval '1 second'
	  WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, sale); err != nil {
		t.Fatal(err)
	}
	claimsB, err = fiscalapp.ClaimFiscalPreflight(ctx, pool, "replica-b", 2, time.Minute)
	if err != nil || len(claimsB) != 1 {
		t.Fatalf("expired work not reclaimed: count=%d err=%v", len(claimsB), err)
	}
	if err := fiscalapp.CompleteFiscalPreflight(ctx, pool, "replica-a", claimsA[0], result); !errors.Is(err, fiscalapp.ErrFiscalLeaseLost) {
		t.Fatalf("stale lease completed after takeover: %v", err)
	}
	if err := fiscalapp.CompleteFiscalPreflight(ctx, pool, "replica-b", claimsB[0], result); err != nil {
		t.Fatalf("new owner cannot finish: %v", err)
	}
	var recorded string
	var claimCount int
	if err := pool.QueryRow(ctx, `
	 SELECT last_result_code,claim_count FROM fiscal_dispatch_jobs
	 WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, sale).Scan(&recorded, &claimCount); err != nil {
		t.Fatal(err)
	}
	if recorded != fiscalapp.PreflightIssuerIncomplete || claimCount != 2 {
		t.Fatalf("lease result not persisted: result=%s attempts=%d", recorded, claimCount)
	}
	// Neither a crash nor a takeover is allowed to generate a document.
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM invoices WHERE tenant_id=$1 AND sale_id=$2", tenant, sale).Scan(&invoices); err != nil {
		t.Fatal(err)
	}
	if invoices != 0 {
		t.Fatal("preflight lease unexpectedly created fiscal invoice")
	}
	// Truly concurrent workers must still claim this job exactly once.
	if _, err := pool.Exec(ctx, `
	 UPDATE fiscal_dispatch_jobs SET next_claim_at=now()-interval '1 second'
	 WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, sale); err != nil {
		t.Fatal(err)
	}
	type concurrentLeaseResult struct {
		owner  string
		claims []fiscalapp.FiscalPreflightClaim
		err    error
	}
	results := make(chan concurrentLeaseResult, 2)
	for _, owner := range []string{"worker-one", "worker-two"} {
		go func(owner string) {
			claims, err := fiscalapp.ClaimFiscalPreflight(ctx, pool, owner, 1, time.Minute)
			results <- concurrentLeaseResult{owner, claims, err}
		}(owner)
	}
	claimedTotal := 0
	for range 2 {
		got := <-results
		if got.err != nil {
			t.Fatalf("concurrent claim failed: %v", got.err)
		}
		claimedTotal += len(got.claims)
		for _, claim := range got.claims {
			if err := fiscalapp.CompleteFiscalPreflight(ctx, pool, got.owner, claim, result); err != nil {
				t.Fatal(err)
			}
		}
	}
	if claimedTotal != 1 {
		t.Fatalf("concurrent lease claimed %d times, want 1", claimedTotal)
	}
	// Each lease transition must leave append-only evidence linked to
	// a distinct machine principal, never a staff actor_user_id.
	var claimsLogged, completionsLogged, reclaimsLogged int
	if err := pool.QueryRow(ctx, `
 SELECT
   count(*) FILTER (WHERE e.event_kind='claimed'),
   count(*) FILTER (WHERE e.event_kind='completed'),
   count(*) FILTER (WHERE e.reclaimed_from_token IS NOT NULL)
 FROM fiscal_preflight_audit_events e
 JOIN fiscal_worker_identities w ON w.id=e.worker_identity_id
 WHERE e.tenant_id=$1 AND e.sale_id=$2
   AND w.kind='local_preflight'
 `, tenant, sale).Scan(&claimsLogged, &completionsLogged, &reclaimsLogged); err != nil {
		t.Fatal(err)
	}
	if claimsLogged != 3 || completionsLogged != 2 || reclaimsLogged != 1 {
		t.Fatalf("unexpected fiscal machine audit: claims=%d completed=%d recovered=%d",
			claimsLogged, completionsLogged, reclaimsLogged)
	}
	// Attempting to rewrite an audit event must fail and leave the chain intact.
	if _, err := pool.Exec(ctx, `
 UPDATE fiscal_preflight_audit_events SET result_code='internal_error'
 WHERE tenant_id=$1 AND sale_id=$2
 `, tenant, sale); err == nil {
		t.Fatal("fiscal machine audit history was mutable")
	}
	// Revocation forbids reuse of the old worker even when a future job appears.
	if _, err := pool.Exec(ctx, `
 UPDATE fiscal_worker_identities SET revoked_at=now()
 WHERE owner_name='replica-b'
 `); err != nil {
		t.Fatal(err)
	}
	if err := fiscalapp.RenewFiscalPreflightWorker(ctx, pool, "replica-b"); err == nil {
		t.Fatal("revoked technical identity was renewed")
	}
	// A model that isn't implemented must never enter the NFC-e processing state.
	if _, err := pool.Exec(ctx, `
 UPDATE sale_fiscal_intents SET document_kind='nfe'
 WHERE tenant_id=$1 AND sale_id=$2
 `, tenant, sale); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
 UPDATE fiscal_dispatch_jobs SET next_check_at=now()-interval '1 second'
 WHERE tenant_id=$1 AND sale_id=$2
 `, tenant, sale); err != nil {
		t.Fatal(err)
	}
	if _, err := fiscalapp.ReconcileFiscalDispatchBatch(ctx, pool, 100); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
 SELECT state FROM fiscal_dispatch_jobs WHERE tenant_id=$1 AND sale_id=$2
 `, tenant, sale).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "unsupported_model" {
		t.Fatalf("NF-e 55 incorrectly allowed: %s", state)
	}
}
