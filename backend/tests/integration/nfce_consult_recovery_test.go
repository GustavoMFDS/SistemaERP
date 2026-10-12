//go:build integration

package integration_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	fiscal "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Run only against a fresh disposable PostgreSQL database with migrations 1-46.
// Tests never invoke any SEFAZ implementation.
func TestNFCeConsultationRecoveryIsTenantScopedAndFenced(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" || os.Getenv("TEST_FISCAL_DISPOSABLE_DB") != "1" {
		t.Skip("disposable PostgreSQL is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var tenantA, actor, cnpjA string
	if err := pool.QueryRow(ctx, `
		SELECT utr.tenant_id::text,utr.user_id::text,c.cnpj
		FROM user_tenant_roles utr
		JOIN roles r ON r.id=utr.role_id AND r.name='admin'
		JOIN companies c ON c.id=utr.tenant_id
		ORDER BY utr.user_id LIMIT 1
	`).Scan(&tenantA, &actor, &cnpjA); err != nil {
		t.Fatal(err)
	}
	tenantB := ""
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies(legal_name,cnpj,address_state)
		VALUES($1,'11222333000181','MG') RETURNING id::text
	`, "Consulta NFC-e Tenant B "+uuid.NewString()).Scan(&tenantB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_tenants(user_id,tenant_id) VALUES($1,$2)
	`, actor, tenantB); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE companies SET address_state='MG' WHERE id=$1", tenantA); err != nil {
		t.Fatal(err)
	}
	makeSubmitted := func(tenant, cnpj string, number int64) string {
		t.Helper()
		var registerID, sessionID, saleID, invoiceID string
		if err := pool.QueryRow(ctx, `
			INSERT INTO cash_registers(tenant_id,name,active)
			VALUES($1,$2,true) RETURNING id::text
		`, tenant, "Recovery Fixture "+uuid.NewString()).Scan(&registerID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,opening_amount,status)
			VALUES($1,$2,$3,0,'open') RETURNING id::text
		`, tenant, registerID, actor).Scan(&sessionID); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO sales(tenant_id,cash_session_id,status,subtotal,discount_value,total,profit_estimated,created_by_user_id)
			VALUES($1,$2,'finalized',10,0,10,5,$3) RETURNING id::text
		`, tenant, sessionID, actor).Scan(&saleID); err != nil {
			t.Fatal(err)
		}
		now := time.Date(2026, 10, 12, 10, 30, 0, 0, time.UTC)
		key, err := fisc.BuildNFCeAccessKey(fisc.NFCeAccessKeyInput{
			UF: "MG", IssuedAt: now, CNPJ: cnpj, Series: 151, Number: number,
			NumericCode: "12345678", EmissionType: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO invoices(
				tenant_id,sale_id,company_id,status,created_by_user_id,
				model,series,document_number,environment,access_key,
				emission_type,numeric_code,access_key_check_digit,issued_at)
			VALUES($1,$2,$1,'submitted',$3,65,151,$4,'homologation',$5,1,'12345678',$6,$7)
			RETURNING id::text
		`, tenant, saleID, actor, number, key, int(key[len(key)-1]-'0'), now).Scan(&invoiceID); err != nil {
			t.Fatal(err)
		}
		return invoiceID
	}
	invoiceA := makeSubmitted(tenantA, cnpjA, 101)
	invoiceB := makeSubmitted(tenantB, "11222333000181", 102)

	if added, err := fiscal.PlanNFCeSubmittedConsultations(ctx, pool); err != nil || added != 2 {
		t.Fatalf("initial recovery plan added=%d err=%v", added, err)
	}
	if added, err := fiscal.PlanNFCeSubmittedConsultations(ctx, pool); err != nil || added != 0 {
		t.Fatalf("plan is not idempotent: added=%d err=%v", added, err)
	}

	ownerA := "consult-test-a-" + uuid.NewString()
	ownerB := "consult-test-b-" + uuid.NewString()
	for _, owner := range []string{ownerA, ownerB} {
		if err := fiscal.RegisterFiscalPreflightWorker(ctx, pool, owner); err != nil {
			t.Fatal(err)
		}
	}
	claimsA, err := fiscal.ClaimNFCeSubmittedConsultations(ctx, pool, ownerA, 1, time.Minute)
	if err != nil || len(claimsA) != 1 {
		t.Fatalf("first claim=%+v err=%v", claimsA, err)
	}
	claimsB, err := fiscal.ClaimNFCeSubmittedConsultations(ctx, pool, ownerB, 2, time.Minute)
	if err != nil || len(claimsB) != 1 {
		t.Fatalf("second worker duplicated claim: %+v err=%v", claimsB, err)
	}
	if claimsA[0].InvoiceID == claimsB[0].InvoiceID || claimsA[0].LeaseToken == claimsB[0].LeaseToken {
		t.Fatalf("overlapping recovery claims: %+v %+v", claimsA, claimsB)
	}
	for _, claim := range []fiscal.NFCeConsultClaim{claimsA[0], claimsB[0]} {
		if claim.Environment != "homologation" || claim.AttemptCount != 1 {
			t.Fatalf("unexpected claim metadata: %+v", claim)
		}
	}
	if err := fiscal.CompleteNFCeConsultation(ctx, pool, ownerB, claimsA[0], fiscal.NFCeConsultPending, 217); !errors.Is(err, fiscal.ErrNFCeConsultLeaseLost) {
		t.Fatalf("different owner must be rejected: %v", err)
	}
	if err := fiscal.CompleteNFCeConsultation(ctx, pool, ownerA, claimsA[0], fiscal.NFCeConsultNotFound, 217); err != nil {
		t.Fatal(err)
	}
	if err := fiscal.CompleteNFCeConsultation(ctx, pool, ownerA, claimsA[0], fiscal.NFCeConsultNotFound, 217); !errors.Is(err, fiscal.ErrNFCeConsultLeaseLost) {
		t.Fatalf("replayed completion must fail: %v", err)
	}
	if err := fiscal.CompleteNFCeConsultation(ctx, pool, ownerB, claimsB[0], fiscal.NFCeConsultManualReview, 110); err != nil {
		t.Fatal(err)
	}
	var statusA, statusB string
	if err := pool.QueryRow(ctx, "SELECT status FROM invoices WHERE tenant_id=$1 AND id=$2", tenantA, invoiceA).
		Scan(&statusA); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT status FROM invoices WHERE tenant_id=$1 AND id=$2", tenantB, invoiceB).
		Scan(&statusB); err != nil {
		t.Fatal(err)
	}
	if statusA != "submitted" || statusB != "submitted" {
		t.Fatalf("consultation results wrongly changed invoice status: A=%s B=%s", statusA, statusB)
	}
	var code int
	var last, state string
	if err := pool.QueryRow(ctx, `
		SELECT last_status_code,last_result_code,state FROM nfce_consult_recovery_jobs
		WHERE tenant_id=$1 AND invoice_id=$2
	`, claimsA[0].TenantID, claimsA[0].InvoiceID).Scan(&code, &last, &state); err != nil {
		t.Fatal(err)
	}
	if code != 217 || last != fiscal.NFCeConsultNotFound || state != "awaiting_consultation" {
		t.Fatalf("unexpected A recovery: %d %s %s", code, last, state)
	}
	if err := pool.QueryRow(ctx, `
		SELECT state FROM nfce_consult_recovery_jobs WHERE tenant_id=$1 AND invoice_id=$2
	`, claimsB[0].TenantID, claimsB[0].InvoiceID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "manual_review" {
		t.Fatalf("manual review did not remain blocked: %s", state)
	}
	if claims, err := fiscal.ClaimNFCeSubmittedConsultations(ctx, pool, ownerA, 5, time.Minute); err != nil || len(claims) != 0 {
		t.Fatalf("undue immediate retries after release: %+v %v", claims, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE invoices SET status='rejected',rejection_code='999',rejection_message='fixture'
		WHERE tenant_id=$1 AND id=$2
	`, claimsA[0].TenantID, claimsA[0].InvoiceID); err != nil {
		t.Fatal(err)
	}
	if resolved, err := fiscal.ResolveNFCeConsultationPlans(ctx, pool); err != nil || resolved != 1 {
		t.Fatalf("resolve external final state=%d err=%v", resolved, err)
	}
	if again, err := fiscal.PlanNFCeSubmittedConsultations(ctx, pool); err != nil || again != 0 {
		t.Fatalf("resolved work requeued=%d err=%v", again, err)
	}
	for _, tenant := range []string{tenantA, tenantB} {
		var count int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM nfce_consult_recovery_jobs WHERE tenant_id=$1
		`, tenant).Scan(&count); err != nil || count != 1 {
			t.Fatalf("tenant %s recovery isolation: %d %v", tenant, count, err)
		}
	}
}
