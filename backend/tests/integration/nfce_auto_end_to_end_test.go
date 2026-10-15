//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/httpapi"
	"github.com/example/sistemaemgo/internal/httpapi/handlers"
	"github.com/example/sistemaemgo/internal/modules"
	"github.com/golang-jwt/jwt/v5"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	fiscal "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	fiscinfra "github.com/example/sistemaemgo/internal/modules/fiscal/infrastructure"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	salesinfra "github.com/example/sistemaemgo/internal/modules/sales/infrastructure"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Renderer stub isolates the service's database-level protocol/archive
// checks from thermal HTML generation in this integration test.
type testReceiptRenderer struct{}

func (testReceiptRenderer) Render(_ fisc.NFCeReservation, _ []byte) ([]byte, error) {
	return []byte("<html><body>MOCK DANFE</body></html>"), nil
}

func TestAutoHomologationSaleToSubmittedToAuthorizedWithFakeSEFAZ(t *testing.T) {
	if os.Getenv("TEST_FISCAL_DISPOSABLE_DB") != "1" ||
		os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("fresh disposable PostgreSQL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var tenant, actor string
	if err := pool.QueryRow(ctx, `
		SELECT ur.tenant_id::text,ur.user_id::text
		FROM user_tenant_roles ur JOIN roles r
		  ON r.id=ur.role_id AND r.name='admin'
		ORDER BY ur.user_id LIMIT 1
	`).Scan(&tenant, &actor); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE companies SET ie='110042490114',crt='1',
		  address_street='Rua Teste',address_number='12',
		  address_neighborhood='Centro',address_city='Uberlandia',
		  address_city_code='3170206',address_state='MG',
		  address_zip='38400000' WHERE id=$1
	`, tenant); err != nil {
		t.Fatal(err)
	}
	var product string
	if err := pool.QueryRow(ctx, `
		INSERT INTO products(tenant_id,sku,name,unit,cost_price,
		  price_cash,min_stock,active,ncm)
		VALUES($1,$2,'Produto Simulado','UN',5,10,0,true,'61091000')
		RETURNING id::text
	`, tenant, "AUTO-NFCE-"+uuid.NewString()).Scan(&product); err != nil {
		t.Fatal(err)
	}
	fiscalRepo := fiscinfra.NewFiscalRepo(pool)
	uow := db.NewPgxUnitOfWork(pool)
	tx, err := uow.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := fiscalRepo.UpsertProductFiscalProfile(ctx, tx, tenant, actor, fisc.ProductFiscalProfile{
		TenantID: tenant, ProductID: product, CFOP: "5102", ICMSOrigin: "0",
		ICMSRegime: "csosn", ICMSCode: "102", PISCST: "49", COFINSCST: "49",
		ReferenceVersion: "auto-hom-integration-synthetic",
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	cert := "secret://test/never-live"
	if _, err := pool.Exec(ctx, `
		INSERT INTO nfce_configs(tenant_id,environment,series,certificate_secret_ref,updated_at)
		VALUES($1,'homologation',163,$2,now()-interval '1 minute')
		ON CONFLICT (tenant_id) DO UPDATE SET
		  environment='homologation',series=163,
		  certificate_secret_ref=excluded.certificate_secret_ref,
		  updated_at=now()-interval '1 minute'
	`, tenant, cert); err != nil {
		t.Fatal(err)
	}
	var register, session, sale, saleItem string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_registers(tenant_id,name,active)
		VALUES($1,$2,true) RETURNING id::text
	`, tenant, "Auto Hom Test "+uuid.NewString()).Scan(&register); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,
		  opening_amount,status)
		VALUES($1,$2,$3,0,'open') RETURNING id::text
	`, tenant, register, actor).Scan(&session); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO sales(tenant_id,cash_session_id,status,subtotal,
		  discount_value,total,profit_estimated,created_by_user_id,finalized_at)
		VALUES($1,$2,'finalized',10,0,10,5,$3,now())
		RETURNING id::text
	`, tenant, session, actor).Scan(&sale); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO sale_items(tenant_id,sale_id,product_id,qty,
		  unit_price,discount_value,subtotal,cost_unit)
		VALUES($1,$2,$3,1,10,0,10,5) RETURNING id::text
	`, tenant, sale, product).Scan(&saleItem); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO payments(tenant_id,sale_id,method,amount)
		VALUES($1,$2,'cash',10)
	`, tenant, sale); err != nil {
		t.Fatal(err)
	}

	if _, err := fiscal.PlanNFCeAutomaticHomologation(ctx, pool); err != nil {
		t.Fatal(err)
	}
	owner := "auto-hom-e2e-" + uuid.NewString()
	if err := fiscal.RegisterFiscalPreflightWorker(ctx, pool, owner); err != nil {
		t.Fatal(err)
	}
	claims, err := fiscal.ClaimNFCeAutomaticHomologation(ctx, pool, owner, 5, 3*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var job fiscal.NFCeAutoClaim
	for _, candidate := range claims {
		if candidate.SaleID == sale {
			job = candidate
		}
	}
	if job.SaleID == "" {
		t.Fatalf("new sale not claimed: %+v", claims)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := fiscal.NewFiscalService(uow, fiscalRepo,
		salesinfra.NewSalesRepo(pool), invinfra.NewProductsRepo(pool),
		audit.New(pool, logger), validator.New(), logger)
	svc.SetNFCeDocumentBuilder(fakeNFCeDocumentBuilder{})
	svc.SetNFCeXMLSigner(fakeNFCeSigner{})
	svc.SetNFCeSchemaValidator(fakeCancellationValidator{})
	svc.SetNFCeProcessedDocumentBuilder(fakeProcessedDocumentBuilder{})
	svc.SetNFCeDANFERenderer(testReceiptRenderer{})
	remote := &fakeRemoteAuthorizer{authorizeOut: fisc.NFCeRemoteOutcome{
		StatusCode: 103, Reason: "recebido; resultado incerto",
	}}
	svc.SetNFCeRemoteAuthorizer(remote)

	state, err := fiscal.ProcessNFCeAutoHomologationClaim(ctx, pool, svc, owner, job)
	if err != nil || state != fiscal.NFCeAutoAwaiting {
		t.Fatalf("failed automatic reserve/taxes/sign/send: state=%s err=%v", state, err)
	}
	if remote.authorizeCalls != 1 || remote.consultCalls != 0 {
		t.Fatalf("first submission must authorize once: %+v", remote)
	}
	if err := fiscal.CompleteNFCeAutomaticHomologation(
		ctx, pool, owner, job, state, "awaiting_sefaz"); err != nil {
		t.Fatal(err)
	}
	var invoice, accessKey, invoiceStatus string
	if err := pool.QueryRow(ctx, `
		SELECT id::text,access_key,status FROM invoices
		WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, sale).Scan(&invoice, &accessKey, &invoiceStatus); err != nil {
		t.Fatal(err)
	}
	if invoiceStatus != "submitted" {
		t.Fatalf("not submitted: %s", invoiceStatus)
	}
	// A crash between SOAP delivery and local queue completion may leave a
	// "queued" row with its final lease expired. Recovery must select
	// CONSULT, never POST Authorize again, even after attempt exhaustion.
	if _, err := pool.Exec(ctx, `
		UPDATE nfce_auto_homologation_jobs SET state='queued',
		  attempt_count=12,lease_token=gen_random_uuid(),
		  lease_owner=$3,lease_until=now()-interval '1 second'
		WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, sale, owner); err != nil {
		t.Fatal(err)
	}
	if n, err := fiscal.ReconcileNFCeAutomaticHomologationJobs(ctx, pool); err != nil || n != 1 {
		t.Fatalf("submitted invoice not reconciled to CONSULT: %d %v", n, err)
	}
	var recoveredQueueState string
	if err := pool.QueryRow(ctx, `
		SELECT state FROM nfce_auto_homologation_jobs
		WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, sale).Scan(&recoveredQueueState); err != nil || recoveredQueueState != "awaiting_sefaz" {
		t.Fatalf("crash recovery lost submitted document: %q %v",
			recoveredQueueState, err)
	}
	if remote.authorizeCalls != 1 {
		t.Fatal("recovery caused a duplicate SEFAZ submission")
	}

	// The exact cashier HTTP route cannot expose a submitted/uncertain note.
	// The final authorized path below uses the same JWT + tenant-scoped route.
	webCfg := config.Config{
		Env: "dev", FiscalProvider: "mvp",
		JWTIssuer:      "cashier-xml-integration",
		JWTSecret:      "disposable-cashier-xml-integration-secret",
		AccessTokenTTL: time.Hour,
	}
	api := httpapi.NewRouter(webCfg, modules.New(webCfg, pool, nil, logger), logger)
	tokenFor := func(tenantID string) string {
		t.Helper()
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"iss": webCfg.JWTIssuer, "sub": actor, "tenant_id": tenantID,
			"typ": "access", "iat": time.Now().Unix(),
			"exp": time.Now().Add(time.Hour).Unix(),
		}).SignedString([]byte(webCfg.JWTSecret))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	authToken := tokenFor(tenant)
	getCashierXML := func(saleID, token string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet,
			"/api/v1/sales/"+saleID+"/fiscal-xml", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		api.ServeHTTP(w, req)
		return w
	}
	if response := getCashierXML(sale, ""); response.Code != http.StatusUnauthorized {
		t.Fatalf("XML without JWT: HTTP %d", response.Code)
	}
	if response := getCashierXML(sale, authToken); response.Code != http.StatusConflict {
		t.Fatalf("XML of submitted invoice exposed: HTTP %d body=%s",
			response.Code, response.Body.String())
	}
	pendingStatusReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/sales/"+sale+"/fiscal-status", nil)
	pendingStatusReq.Header.Set("Authorization", "Bearer "+authToken)
	pendingStatusResponse := httptest.NewRecorder()
	api.ServeHTTP(pendingStatusResponse, pendingStatusReq)
	if pendingStatusResponse.Code != http.StatusOK ||
		!bytes.Contains(pendingStatusResponse.Body.Bytes(), []byte(`"automatic_state":"awaiting_sefaz"`)) ||
		bytes.Contains(pendingStatusResponse.Body.Bytes(), []byte(`"authorized":true`)) {
		t.Fatalf("cashier must show awaiting consultation, not authorization: HTTP %d body=%s",
			pendingStatusResponse.Code, pendingStatusResponse.Body.String())
	}
	if response := getCashierXML(sale, tokenFor(uuid.NewString())); response.Code == http.StatusOK {
		t.Fatal("cross-tenant cashier retrieved XML")
	}
	var taxItems int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM invoice_item_tax_calculations
		WHERE tenant_id=$1 AND invoice_id=$2
	`, tenant, invoice).Scan(&taxItems); err != nil || taxItems != 1 {
		t.Fatalf("missing copied configured fiscal item calc: %d %v", taxItems, err)
	}
	// The invoice can be marked authorized by a faulty/manual database
	// operation while no SEFAZ archive exists. Even then the Fiscal-page
	// renderer must refuse to print. Restore submitted before consultation.
	if _, err := pool.Exec(ctx, `
		UPDATE invoices SET status='authorized',
		  authorization_protocol='fixture-not-real',authorized_at=now()
		WHERE tenant_id=$1 AND id=$2 AND status='submitted'
	`, tenant, invoice); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.RenderNFCeDANFE(ctx, tenant, invoice, 80); err == nil {
		t.Fatal("authorized status without protocol/nfeProc archive printed DANFE")
	}
	manager := handlers.NewSaleFiscalHandler(pool, svc, audit.New(pool, logger))
	unarchivedPage, err := manager.QueryObligations(ctx, tenant, 100, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, obligation := range unarchivedPage.Items {
		if obligation.SaleID != sale {
			continue
		}
		found = true
		if obligation.Authorized || obligation.ProcessedAvailable ||
			obligation.Status != "missing_processed_xml_review" {
			t.Fatalf("manager showed a non-archived note as authorized: %+v", obligation)
		}
	}
	if !found {
		t.Fatal("manager did not return synthetic sale")
	}

	if _, err := pool.Exec(ctx, `
		UPDATE invoices SET status='submitted',
		  authorization_protocol=NULL,authorized_at=NULL
		WHERE tenant_id=$1 AND id=$2 AND status='authorized'
	`, tenant, invoice); err != nil {
		t.Fatal(err)
	}
	// The first send is acknowledged but inconclusive; only Consult may
	// reconcile it. A second POST to Authorize is never issued.
	if _, err := fiscal.PlanNFCeSubmittedConsultations(ctx, pool); err != nil {
		t.Fatal(err)
	}
	consultWorker := "auto-consult-e2e-" + uuid.NewString()
	if err := fiscal.RegisterFiscalPreflightWorker(ctx, pool, consultWorker); err != nil {
		t.Fatal(err)
	}
	consultJobs, err := fiscal.ClaimNFCeSubmittedConsultations(ctx, pool, consultWorker, 10, 3*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var recovery fiscal.NFCeConsultClaim
	for _, candidate := range consultJobs {
		if candidate.InvoiceID == invoice {
			recovery = candidate
		}
	}
	if recovery.InvoiceID == "" {
		t.Fatalf("not scheduled for consult: %+v", consultJobs)
	}
	remote.consultOut = fisc.NFCeRemoteOutcome{
		AccessKey: accessKey, StatusCode: 100, Reason: "autorizado",
		FinalStatus: fisc.NFCeStatusAuthorized,
		Protocol:    "131260000009991", ReceivedAt: time.Now().UTC(),
		ProtocolXML: []byte("<protNFe>fake protocol from simulated SEFAZ</protNFe>"),
	}
	out, err := svc.ExecuteNFCeConsultationClaim(ctx, pool, consultWorker, actor, recovery)
	if err != nil || !out.Authorized() {
		t.Fatalf("simulated consult not applied: %+v %v", out, err)
	}
	if remote.authorizeCalls != 1 || remote.consultCalls != 1 {
		t.Fatalf("unsafe resend or missing consultation: %+v", remote)
	}
	if _, err := fiscal.ObserveAuthorizedAutomaticNFCeJob(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var jobState, finalInvoice string
	if err := pool.QueryRow(ctx, `
		SELECT j.state,i.status FROM nfce_auto_homologation_jobs j
		JOIN invoices i ON i.tenant_id=j.tenant_id AND i.sale_id=j.sale_id
		WHERE j.tenant_id=$1 AND j.sale_id=$2
	`, tenant, sale).Scan(&jobState, &finalInvoice); err != nil {
		t.Fatal(err)
	}
	if finalInvoice != "authorized" || jobState != "authorized" {
		t.Fatalf("not authorized with completed queue: invoice=%s job=%s", finalInvoice, jobState)
	}
	authorizedPage, err := manager.QueryObligations(ctx, tenant, 100, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, obligation := range authorizedPage.Items {
		if obligation.SaleID != sale {
			continue
		}
		found = true
		if !obligation.Authorized || !obligation.ProcessedAvailable ||
			obligation.Status != "authorized" || obligation.AutomaticState == nil ||
			*obligation.AutomaticState != "authorized" ||
			obligation.ConsultationState == nil ||
			*obligation.ConsultationState != "resolved" {
			t.Fatalf("manager obscured recovered authorization: %+v", obligation)
		}
	}
	if !found {
		t.Fatal("manager did not return archived sale")
	}

	// Only this complete, tenant-scoped and hash-verified nfeProc can be
	// exported. Neither copying paper nor printing a DANFE is required.
	response := getCashierXML(sale, authToken)
	if response.Code != http.StatusOK ||
		!strings.HasPrefix(response.Header().Get("Content-Type"), "application/xml") ||
		!strings.Contains(response.Header().Get("Content-Disposition"), "-procNFe.xml") ||
		response.Header().Get("Cache-Control") != "private, no-store" ||
		response.Body.Len() == 0 {
		t.Fatalf("authorized cashier XML export failed: HTTP %d body=%s",
			response.Code, response.Body.String())
	}
	if _, archivedXML, err := svc.DownloadAuthorizedNFCeProcessedXML(ctx, tenant, invoice); err != nil || !bytes.Equal(archivedXML, response.Body.Bytes()) {
		t.Fatalf("cashier XML differed from hash-validated archive: %v", err)
	}
	var originalHash string
	if err := pool.QueryRow(ctx, `
		SELECT sha256 FROM invoice_authorized_xml_files
		WHERE tenant_id=$1 AND invoice_id=$2
	`, tenant, invoice).Scan(&originalHash); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE invoice_authorized_xml_files SET sha256=repeat('0',64)
		WHERE tenant_id=$1 AND invoice_id=$2
	`, tenant, invoice); err != nil {
		t.Fatal(err)
	}
	if response := getCashierXML(sale, authToken); response.Code != http.StatusConflict ||
		bytes.Contains(response.Body.Bytes(), []byte("<nfeProc")) {
		t.Fatalf("corrupted archive should be rejected: HTTP %d body=%s",
			response.Code, response.Body.String())
	}
	// A stored document whose SHA-256 does not match is not "ready for
	// printing" in *any* backend route or management view, even though
	// invoices.status still says authorized.
	if _, _, err := svc.RenderNFCeDANFE(ctx, tenant, invoice, 80); err == nil {
		t.Fatal("corrupted nfeProc rendered a DANFE")
	}
	corruptedPage, err := manager.QueryObligations(ctx, tenant, 100, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, row := range corruptedPage.Items {
		if row.SaleID != sale {
			continue
		}
		found = true
		if row.Authorized || row.ProcessedAvailable ||
			row.Status != "missing_processed_xml_review" {
			t.Fatalf("corrupted nfeProc was represented as authorized: %+v", row)
		}
	}
	if !found {
		t.Fatal("manager hid corrupted sale")
	}
	// A document can become corrupted AFTER the authorization was observed.
	// Both locally resolved queues must be reopened for human review rather
	// than continuing to advertise an intact government authorization.
	if n, err := fiscal.ReconcileNFCeArchivedIntegrity(ctx, pool); err != nil || n != 2 {
		t.Fatalf("archive drift did not escalate both completed queues: %d %v", n, err)
	}
	var autoDriftState, consultDriftState, stillInvoiceStatus string
	if err := pool.QueryRow(ctx, `
		SELECT a.state,c.state,i.status
		FROM invoices i
		JOIN nfce_auto_homologation_jobs a
		  ON a.tenant_id=i.tenant_id AND a.sale_id=i.sale_id
		JOIN nfce_consult_recovery_jobs c
		  ON c.tenant_id=i.tenant_id AND c.invoice_id=i.id
		WHERE i.tenant_id=$1 AND i.id=$2
	`, tenant, invoice).Scan(&autoDriftState,&consultDriftState,&stillInvoiceStatus); err != nil {
		t.Fatal(err)
	}
	if autoDriftState!="manual_review" || consultDriftState!="manual_review" ||
		stillInvoiceStatus!="authorized" {
		t.Fatalf("archive drift must affect queues only: %s %s %s",
			autoDriftState,consultDriftState,stillInvoiceStatus)
	}
	if n,err:=fiscal.ReconcileNFCeArchivedIntegrity(ctx,pool);err!=nil || n!=0 {
		t.Fatalf("integrity sweep not idempotent: %d %v",n,err)
	}
	statusReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/sales/"+sale+"/fiscal-status", nil)
	statusReq.Header.Set("Authorization", "Bearer "+authToken)
	statusResponse := httptest.NewRecorder()
	api.ServeHTTP(statusResponse, statusReq)
	if statusResponse.Code != http.StatusOK ||
		bytes.Contains(statusResponse.Body.Bytes(), []byte(`"authorized":true`)) ||
		bytes.Contains(statusResponse.Body.Bytes(), []byte(`"printable":true`)) {
		t.Fatalf("cashier status falsely accepted altered archive: %d %s",
			statusResponse.Code, statusResponse.Body.String())
	}
	if _, err := pool.Exec(ctx, `
		UPDATE nfce_auto_homologation_jobs SET state='queued'
		WHERE tenant_id=$1 AND sale_id=$2
	`, tenant, sale); err != nil {
		t.Fatal(err)
	}
	if n, err := fiscal.ObserveAuthorizedAutomaticNFCeJob(ctx, pool); err != nil || n != 0 {
		t.Fatalf("corrupt archive completed queue: %d %v", n, err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE invoice_authorized_xml_files SET sha256=$3
		WHERE tenant_id=$1 AND invoice_id=$2
	`, tenant, invoice, originalHash); err != nil {
		t.Fatal(err)
	}
	if n, err := fiscal.ObserveAuthorizedAutomaticNFCeJob(ctx, pool); err != nil || n != 1 {
		t.Fatalf("valid restored archive not observed: %d %v", n, err)
	}
	if n,err:=fiscal.ResolveNFCeConsultationPlans(ctx,pool);err!=nil || n!=1 {
		t.Fatalf("valid restored archive did not reconcile consultation: %d %v",n,err)
	}

	if _, rendered, err := svc.RenderNFCeDANFE(ctx, tenant, invoice, 80); err != nil || len(rendered) == 0 {
		t.Fatalf("processed authorized invoice should be printable: %v", err)
	}
	var archived, audited int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM invoice_authorized_xml_files
		WHERE tenant_id=$1 AND invoice_id=$2
	`, tenant, invoice).Scan(&archived); err != nil || archived != 1 {
		t.Fatalf("processed XML not archived: %d %v", archived, err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE tenant_id=$1 AND resource_id=$2
		  AND action='fiscal.nfce.authorized'
	`, tenant, invoice).Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("no unique SEFAZ authorisation audit: %d %v", audited, err)
	}
}
