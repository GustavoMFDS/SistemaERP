//go:build integration

package integration_test

import (
	"context"
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
