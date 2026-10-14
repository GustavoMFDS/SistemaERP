//go:build integration

package integration_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	fiscal "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests require the explicitly disposable database. No SEFAZ or live
// certificate is used. The opt-in worker does not start during Go tests.
func TestAutomaticNFCeHomologationPlansOnlyNewConfiguredSales(t *testing.T) {
	if os.Getenv("TEST_FISCAL_DISPOSABLE_DB") != "1" ||
		os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires disposable PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var tenant, actor string
	if err := pool.QueryRow(ctx, `
		SELECT utr.tenant_id::text,utr.user_id::text
		FROM user_tenant_roles utr
		JOIN roles r ON r.id=utr.role_id AND r.name='admin'
		LIMIT 1
	`).Scan(&tenant, &actor); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO nfce_configs(tenant_id,environment,series,certificate_secret_ref,updated_at)
		VALUES($1,'homologation',2,'secret://synthetic/test',now()-interval '2 hours')
		ON CONFLICT (tenant_id) DO UPDATE SET
		  environment='homologation',certificate_secret_ref='secret://synthetic/test',
		  updated_at=now()-interval '2 hours'
	`, tenant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE companies SET address_state='MG' WHERE id=$1
	`, tenant); err != nil {
		t.Fatal(err)
	}
	var register, session string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_registers(tenant_id,name,active)
		VALUES($1,$2,true) RETURNING id::text
	`, tenant, "Auto NFC-e "+uuid.NewString()).Scan(&register); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,opening_amount,status)
		VALUES($1,$2,$3,0,'open') RETURNING id::text
	`, tenant, register, actor).Scan(&session); err != nil {
		t.Fatal(err)
	}
	makeSale := func(finalizedAt time.Time) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO sales(
			  tenant_id,cash_session_id,status,subtotal,discount_value,
			  total,profit_estimated,created_by_user_id,finalized_at)
			VALUES($1,$2,'finalized',10,0,10,4,$3,$4)
			RETURNING id::text
		`, tenant, session, actor, finalizedAt).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	oldSale := makeSale(time.Now().Add(-3 * time.Hour))
	newSale := makeSale(time.Now())
	if _, err := fiscal.PlanNFCeAutomaticHomologation(ctx, pool); err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		sale string
		want int
	}{
		{oldSale, 0}, {newSale, 1},
	} {
		var count int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM nfce_auto_homologation_jobs
			WHERE tenant_id=$1 AND sale_id=$2
		`, tenant, sample.sale).Scan(&count); err != nil || count != sample.want {
			t.Fatalf("unexpected auto planning old/new %s count %d: %v", sample.sale, count, err)
		}
	}
	if _, err := fiscal.PlanNFCeAutomaticHomologation(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var duplicates int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM nfce_auto_homologation_jobs
		WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, newSale).Scan(&duplicates); err != nil || duplicates != 1 {
		t.Fatalf("duplicate auto job: %d %v", duplicates, err)
	}
	worker := "nfce-auto-test-" + uuid.NewString()
	other := "nfce-auto-test-" + uuid.NewString()
	for _, owner := range []string{worker, other} {
		if err := fiscal.RegisterFiscalPreflightWorker(ctx, pool, owner); err != nil {
			t.Fatal(err)
		}
	}
	claims, err := fiscal.ClaimNFCeAutomaticHomologation(ctx, pool, worker, 1, 2*time.Minute)
	if err != nil || len(claims) != 1 || claims[0].SaleID != newSale {
		t.Fatalf("first automatic job claim: %+v %v", claims, err)
	}
	otherClaims, err := fiscal.ClaimNFCeAutomaticHomologation(ctx, pool, other, 1, 2*time.Minute)
	if err != nil || len(otherClaims) != 0 {
		t.Fatalf("duplicate worker claimed same sale: %+v %v", otherClaims, err)
	}
	claim := claims[0]
	if err := fiscal.CompleteNFCeAutomaticHomologation(ctx, pool, other, claim,
		fiscal.NFCeAutoQueued, "retry"); !errors.Is(err, fiscal.ErrNFCeAutoLeaseLost) {
		t.Fatalf("unowned claim completed: %v", err)
	}
	if err := fiscal.CompleteNFCeAutomaticHomologation(ctx, pool, worker, claim,
		fiscal.NFCeAutoAwaiting, "awaiting_sefaz"); !errors.Is(err, fiscal.ErrNFCeAutoLeaseLost) {
		t.Fatalf("cannot mark awaiting_sefaz without submitted invoice: %v", err)
	}
	if err := fiscal.CompleteNFCeAutomaticHomologation(ctx, pool, worker, claim,
		fiscal.NFCeAutoAuthorized, "authorized"); !errors.Is(err, fiscal.ErrNFCeAutoLeaseLost) {
		t.Fatalf("cannot mark authorized without archived protocol: %v", err)
	}
	if err := fiscal.CompleteNFCeAutomaticHomologation(ctx, pool, worker, claim,
		fiscal.NFCeAutoQueued, "retry"); err != nil {
		t.Fatal(err)
	}
	if err := fiscal.CompleteNFCeAutomaticHomologation(ctx, pool, worker, claim,
		fiscal.NFCeAutoQueued, "retry"); !errors.Is(err, fiscal.ErrNFCeAutoLeaseLost) {
		t.Fatalf("replay claim completed: %v", err)
	}
	var state string
	if err := pool.QueryRow(ctx, `
		SELECT state FROM nfce_auto_homologation_jobs
		WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, newSale).Scan(&state); err != nil || state != fiscal.NFCeAutoQueued {
		t.Fatalf("unexpected retry state: %q %v", state, err)
	}
	// Simulate a worker crash on attempt 12 while it held a live lease.
	// The maintenance pass must not steal that lease or retry the sale.
	if _, err := pool.Exec(ctx, `
		UPDATE nfce_auto_homologation_jobs
		SET attempt_count=12,lease_token=gen_random_uuid(),
			lease_owner=$3,lease_until=now()+interval '2 minutes'
		WHERE tenant_id=$1 AND sale_id=$2 AND state='queued'
	`, tenant, newSale, worker); err != nil {
		t.Fatal(err)
	}
	if updated, err := fiscal.ReconcileNFCeAutomaticHomologationJobs(ctx, pool); err != nil || updated != 0 {
		t.Fatalf("active lease must not be stolen: %d %v", updated, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE nfce_auto_homologation_jobs
		SET lease_until=now()-interval '1 second'
		WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, newSale); err != nil {
		t.Fatal(err)
	}
	if updated, err := fiscal.ReconcileNFCeAutomaticHomologationJobs(ctx, pool); err != nil || updated != 1 {
		t.Fatalf("expired final lease not escalated: %d %v", updated, err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT state FROM nfce_auto_homologation_jobs
		WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, newSale).Scan(&state); err != nil || state != fiscal.NFCeAutoManual {
		t.Fatalf("exhausted state=%q %v", state, err)
	}
	if updated, err := fiscal.ReconcileNFCeAutomaticHomologationJobs(ctx, pool); err != nil || updated != 0 {
		t.Fatalf("reconciliation not idempotent: %d %v", updated, err)
	}
	// Queue state can never fabricate an invoice or an SEFAZ response.
	var invoices int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM invoices
		WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, newSale).Scan(&invoices); err != nil || invoices != 0 {
		t.Fatalf("queue operation issued a false invoice: %d %v", invoices, err)
	}
	if err := fiscal.CompleteNFCeAutomaticHomologation(ctx, pool, worker, claim,
		"fake_authorized", "authorized"); err == nil {
		t.Fatal("unexpected state accepted")
	}
}
