//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	fiscal "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	fiscalinfra "github.com/example/sistemaemgo/internal/modules/fiscal/infrastructure"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// consultationHookAuthorizer simulates a reply arriving after another process
// revokes/invalidates the lease, without contacting SEFAZ.
type consultationHookAuthorizer struct {
	outcome fisc.NFCeRemoteOutcome
	beforeReturn func() error
	consultCalls int
}

func (a *consultationHookAuthorizer) Authorize(
	_ context.Context, _, _, _, _ string, _ int64, _ []byte,
) (fisc.NFCeRemoteOutcome, error) {
	panic("recovery executor must never call Authorize")
}

func (a *consultationHookAuthorizer) Consult(
	_ context.Context, _, _, _, _ string,
) (fisc.NFCeRemoteOutcome, error) {
	a.consultCalls++
	if a.beforeReturn != nil {
		if err := a.beforeReturn(); err != nil {
			return fisc.NFCeRemoteOutcome{}, err
		}
	}
	return a.outcome, nil
}

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
	// Generate a fresh structurally valid CNPJ for every run. Even on an
	// intentionally reused disposable database, keys and tenants must not
	// collide with fixtures from a previous failed test.
	body := fmt.Sprintf("%012d", time.Now().UnixNano()%1_000_000_000_000)
	cnpjB := ""
	for dv := 0; dv < 100; dv++ {
		candidate := fmt.Sprintf("%s%02d", body, dv)
		if fisc.ValidateCNPJ(candidate) == nil {
			cnpjB = candidate
			break
		}
	}
	if cnpjB == "" {
		t.Fatal("could not create valid test CNPJ")
	}
	tenantB := ""
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies(legal_name,cnpj,address_state)
		VALUES($1,$2,'MG') RETURNING id::text
	`, "Consulta NFC-e Tenant B "+uuid.NewString(), cnpjB).Scan(&tenantB); err != nil {
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
	seriesStart := time.Now().UnixNano()%800_000_000 + 100_000
	invoiceA := makeSubmitted(tenantA, cnpjA, seriesStart)
	invoiceB := makeSubmitted(tenantB, cnpjB, seriesStart+1)

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
	if resolved, err := fiscal.ResolveNFCeConsultationPlans(ctx, pool); err != nil || resolved < 1 {
		t.Fatalf("resolve external final state=%d err=%v", resolved, err)
	}
	if again, err := fiscal.PlanNFCeSubmittedConsultations(ctx, pool); err != nil || again != 0 {
		t.Fatalf("resolved work requeued=%d err=%v", again, err)
	}
	for _, item := range []struct{ tenant, invoice string }{
		{tenantA, invoiceA}, {tenantB, invoiceB},
	} {
		var count int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM nfce_consult_recovery_jobs
			WHERE tenant_id=$1 AND invoice_id=$2
		`, item.tenant, item.invoice).Scan(&count); err != nil || count != 1 {
			t.Fatalf("tenant/invoice recovery isolation: %d %v", count, err)
		}
	}
	for _, item := range []struct{ tenant, otherInvoice string }{
		{tenantA, invoiceB}, {tenantB, invoiceA},
	} {
		var count int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM nfce_consult_recovery_jobs
			WHERE tenant_id=$1 AND invoice_id=$2
		`, item.tenant, item.otherInvoice).Scan(&count); err != nil || count != 0 {
			t.Fatalf("cross-tenant recovery leak: %d %v", count, err)
		}
	}


	// The initial SKIP LOCKED claim order is deliberately unspecified; the
	// invoice escalated to manual review is identified by the actual claim.
	// The other invoice was made terminal earlier in this test.
	targetTenant := claimsB[0].TenantID
	targetInvoice := claimsB[0].InvoiceID
	// Simulate a real claimed recovery against a fake Consult-only remote.
	// The fixture has no certificate secrets; a reference is enough for this
	// fully isolated integration test (never a real SEFAZ connection).
	certRef := "secret://integration/recovery/fake-a1"
	if _, err := pool.Exec(ctx, `
		INSERT INTO nfce_configs(tenant_id,environment,series,certificate_secret_ref)
		VALUES($1,'homologation',151,$2)
		ON CONFLICT (tenant_id) DO UPDATE SET
			environment='homologation',certificate_secret_ref=excluded.certificate_secret_ref
	`, targetTenant, certRef); err != nil {
		t.Fatal(err)
	}
	var accessKey string
	if err := pool.QueryRow(ctx,
		"SELECT access_key FROM invoices WHERE tenant_id=$1 AND id=$2",
		targetTenant, targetInvoice).Scan(&accessKey); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO invoice_xml_files(tenant_id,invoice_id,file_name,content,sha256)
		VALUES($1,$2,'fixture-signed.xml',$3,$4)
	`, targetTenant, targetInvoice, []byte("<NFe>signed-in-test-only</NFe>"),
		fmt.Sprintf("%064x", 123)); err != nil {
		t.Fatal(err)
	}
	rescheduleB := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			UPDATE nfce_consult_recovery_jobs SET
				state='awaiting_consultation',next_attempt_at=now()-interval '1 second',
				lease_owner=NULL,lease_token=NULL,lease_until=NULL,
				attempt_count=0 WHERE tenant_id=$1 AND invoice_id=$2
		`, targetTenant, targetInvoice); err != nil {
			t.Fatal(err)
		}
	}
	getBClaim := func() fiscal.NFCeConsultClaim {
		t.Helper()
		jobs, err := fiscal.ClaimNFCeSubmittedConsultations(ctx,pool,ownerB,1,time.Minute)
		if err != nil || len(jobs)!=1 || jobs[0].InvoiceID!=targetInvoice {
			t.Fatalf("recovery claim B: %+v %v",jobs,err)
		}
		return jobs[0]
	}
	rescheduleB()
	remote := &fakeRemoteAuthorizer{consultOut: fisc.NFCeRemoteOutcome{
		AccessKey:accessKey,StatusCode:217,Reason:"nao consta na base", // INCONCLUSIVE
	}}
	svc := fiscal.NewFiscalService(db.NewPgxUnitOfWork(pool),
		fiscalinfra.NewFiscalRepo(pool),nil,nil,audit.New(pool,nil),validator.New(),nil)
	svc.SetNFCeRemoteAuthorizer(remote)
	svc.SetNFCeProcessedDocumentBuilder(fakeProcessedDocumentBuilder{})
	claim := getBClaim()
	if _, err := svc.ExecuteNFCeConsultationClaim(ctx,pool,ownerA,actor,claim);
		!errors.Is(err,fiscal.ErrNFCeConsultLeaseLost) {
		t.Fatalf("wrong lease owner should never call Consult: %v",err)
	}
	if remote.consultCalls!=0 || remote.authorizeCalls!=0 {
		t.Fatal("wrong owner triggered a network adapter")
	}
	// A worker lease does not authorize access as a user from another tenant.
	if _, err := svc.ExecuteNFCeConsultationClaim(ctx,pool,ownerB,uuid.NewString(),claim);
		!errors.Is(err,fiscal.ErrNFCeConsultLeaseLost) {
		t.Fatalf("operator outside tenant must be rejected: %v",err)
	}
	if remote.consultCalls!=0 || remote.authorizeCalls!=0 {
		t.Fatal("unrelated operator triggered SEFAZ consultation")
	}
	pending, err := svc.ExecuteNFCeConsultationClaim(ctx,pool,ownerB,actor,claim)
	if err!=nil || !pending.Pending() || remote.consultCalls!=1 ||
		remote.authorizeCalls!=0 {
		t.Fatalf("not-found consult must remain pending: %+v %v, calls=%d/%d",
			pending,err,remote.consultCalls,remote.authorizeCalls)
	}
	if err := pool.QueryRow(ctx,
		"SELECT status FROM invoices WHERE tenant_id=$1 AND id=$2",
		targetTenant,targetInvoice).Scan(&statusB);err!=nil || statusB!="submitted" {
		t.Fatalf("not-found consult unexpectedly authorized: %s %v",statusB,err)
	}
	// Invalid protocol from a faulty/misconfigured transport must not create
	// an authorization even when a mock supplies a success label.
	rescheduleB()
	claim=getBClaim()
	remote.consultOut=fisc.NFCeRemoteOutcome{
		AccessKey:accessKey,StatusCode:100,
		FinalStatus:fisc.NFCeStatusAuthorized,
		Protocol:"123456789",ReceivedAt:time.Now(),
	}
	if _,err:=svc.ExecuteNFCeConsultationClaim(ctx,pool,ownerB,actor,claim);err==nil {
		t.Fatal("status 100 without genuine protocol XML must be rejected")
	}
	if err := pool.QueryRow(ctx,
		"SELECT status FROM invoices WHERE tenant_id=$1 AND id=$2",
		targetTenant,targetInvoice).Scan(&statusB);err!=nil || statusB!="submitted" {
		t.Fatalf("invalid protocol incorrectly authorized: %s %v",statusB,err)
	}
	// A stale consultation outcome must not be applied to an expired lease.
	rescheduleB()
	claim=getBClaim()
	if _,err := pool.Exec(ctx,`
		UPDATE nfce_consult_recovery_jobs SET lease_until=now()-interval '1 second'
		WHERE tenant_id=$1 AND invoice_id=$2
	`, targetTenant,targetInvoice);err!=nil {t.Fatal(err)}
	remote.consultOut.ProtocolXML=[]byte("<protNFe>fixture-only</protNFe>")
	callsBefore := remote.consultCalls
	if _,err:=svc.ExecuteNFCeConsultationClaim(ctx,pool,ownerB,actor,claim);
		!errors.Is(err,fiscal.ErrNFCeConsultLeaseLost) {
		t.Fatalf("expired lease must be rejected: %v",err)
	}
	if remote.consultCalls!=callsBefore {
		t.Fatal("expired lease invoked Consult")
	}
	// Model a response obtained BEFORE the lease expiry but applied AFTER
	// it. The transaction must recheck the fence at commit, not only before
	// calling Consult. No invoice XML or status may be written.
	rescheduleB()
	claim = getBClaim()
	race := &consultationHookAuthorizer{
		outcome: fisc.NFCeRemoteOutcome{
			AccessKey: accessKey, StatusCode: 100,
			FinalStatus: fisc.NFCeStatusAuthorized,
			Protocol: "123456789",
			ProtocolXML: []byte("<protNFe>race-fixture</protNFe>"),
			ReceivedAt: time.Now().UTC(),
		},
	}
	race.beforeReturn = func() error {
		_, err := pool.Exec(ctx, `
			UPDATE nfce_consult_recovery_jobs SET
				lease_until=now()-interval '1 second'
			WHERE tenant_id=$1 AND invoice_id=$2
		`, targetTenant, targetInvoice)
		return err
	}
	svc.SetNFCeRemoteAuthorizer(race)
	if _, err := svc.ExecuteNFCeConsultationClaim(ctx, pool, ownerB, actor, claim);
		!errors.Is(err, fiscal.ErrNFCeConsultLeaseLost) {
		t.Fatalf("post-network expired lease must be rejected: %v", err)
	}
	if race.consultCalls != 1 {
		t.Fatal("fencing test did not invoke fake Consult")
	}
	if err := pool.QueryRow(ctx,
		"SELECT status FROM invoices WHERE tenant_id=$1 AND id=$2",
		targetTenant, targetInvoice).Scan(&statusB); err != nil || statusB != "submitted" {
		t.Fatalf("stale remote 100 wrongly authorized: %s %v", statusB, err)
	}
	var beforeArchive int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM invoice_authorized_xml_files
		WHERE tenant_id=$1 AND invoice_id=$2
	`, targetTenant, targetInvoice).Scan(&beforeArchive); err != nil || beforeArchive != 0 {
		t.Fatalf("stale remote 100 wrongly archived XML: %d %v", beforeArchive, err)
	}
	svc.SetNFCeRemoteAuthorizer(remote)

	// An unexpired lease + matching 100/protocol enters the existing atomic
	// invoice/protocolXML/nfeProc/archive/audit commit.
	rescheduleB()
	claim=getBClaim()
	authorized,err:=svc.ExecuteNFCeConsultationClaim(ctx,pool,ownerB,actor,claim)
	if err!=nil || !authorized.Authorized() {
		t.Fatalf("claimed consultation authorization failed: %+v %v",authorized,err)
	}
	if remote.authorizeCalls!=0 {
		t.Fatal("consult recovery MUST NOT call Authorize")
	}
	var authStatus,jobState,protocol string
	if err := pool.QueryRow(ctx,`
		SELECT i.status,j.state,i.authorization_protocol
		FROM invoices i JOIN nfce_consult_recovery_jobs j
		ON j.tenant_id=i.tenant_id AND j.invoice_id=i.id
		WHERE i.tenant_id=$1 AND i.id=$2
	`,targetTenant,targetInvoice).Scan(&authStatus,&jobState,&protocol);err!=nil {
		t.Fatal(err)
	}
	if authStatus!="authorized" || jobState!="resolved" || protocol!="123456789" {
		t.Fatalf("authorized + resolved not atomic: %s %s %q",
			authStatus,jobState,protocol)
	}
	var archived int
	if err:=pool.QueryRow(ctx,`
		SELECT count(*) FROM invoice_authorized_xml_files
		WHERE tenant_id=$1 AND invoice_id=$2
	`,targetTenant,targetInvoice).Scan(&archived);err!=nil || archived!=1 {
		t.Fatalf("protocol XML and processed nfeProc not archived: %d %v",archived,err)
	}
	var audited int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs WHERE tenant_id=$1
		  AND entity_id=$2 AND action='fiscal.nfce.authorized'
	`, targetTenant, targetInvoice).Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("authorized NFC-e requires one tenant-scoped audit: %d %v", audited, err)
	}
	if _,err:=svc.ExecuteNFCeConsultationClaim(ctx,pool,ownerB,actor,claim);
		!errors.Is(err,fiscal.ErrNFCeConsultLeaseLost) {
		t.Fatalf("replayed authorization after lease resolution must fail: %v",err)
	}
}
