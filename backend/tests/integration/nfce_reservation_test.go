//go:build integration

package integration_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	fiscapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	fiscinfra "github.com/example/sistemaemgo/internal/modules/fiscal/infrastructure"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	salesinfra "github.com/example/sistemaemgo/internal/modules/sales/infrastructure"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeNFCeDocumentBuilder struct{}

func (fakeNFCeDocumentBuilder) BuildUnsignedLegacyCandidate(
	_ fisc.NFCeDocumentDraft,
) ([]byte, error) {
	return []byte("<NFe><infNFe Id=\"NFe-test\"/></NFe>"), nil
}

func (fakeNFCeDocumentBuilder) BuildOfflineQRCodeSigningPayload(
	_ fisc.NFCeDocumentDraft,
) (string, error) {
	return "offline-qr-payload", nil
}

func (fakeNFCeDocumentBuilder) BuildUnsignedInutilization(
	_ fisc.NFCeInutilizationDraft,
) ([]byte, string, error) {
	return []byte("<inutNFe><infInut Id=\"ID-test\"/></inutNFe>"), "ID-test", nil
}

type fakeNFCeSigner struct{}

func (fakeNFCeSigner) Sign(
	_ context.Context,
	_ string,
	_ string,
	unsignedXML []byte,
) ([]byte, error) {
	return append([]byte(nil), unsignedXML...), nil
}

func (fakeNFCeSigner) SignQRCode(
	_ context.Context,
	_ string,
	_ string,
) (string, error) {
	return "c2lnbmF0dXJl", nil
}

func (fakeNFCeSigner) SignInutilization(
	_ context.Context,
	_ string,
	_ string,
	unsignedXML []byte,
) ([]byte, error) {
	return append([]byte(nil), unsignedXML...), nil
}

type fakeRemoteAuthorizer struct {
	authorizeCalls int
	consultCalls   int
	authorizeOut   fisc.NFCeRemoteOutcome
	consultOut     fisc.NFCeRemoteOutcome
}

func (f *fakeRemoteAuthorizer) Authorize(
	_ context.Context,
	_, _, _, _ string,
	_ int64,
	_ []byte,
) (fisc.NFCeRemoteOutcome, error) {
	f.authorizeCalls++
	return f.authorizeOut, nil
}

func (f *fakeRemoteAuthorizer) Consult(
	_ context.Context,
	_, _, _, _ string,
) (fisc.NFCeRemoteOutcome, error) {
	f.consultCalls++
	return f.consultOut, nil
}

func (f *fakeRemoteAuthorizer) Inutilize(
	_ context.Context,
	_ string,
	_ fisc.NFCeInutilizationDraft,
	requestID string,
	_ []byte,
) (fisc.NFCeInutilizationRemoteResult, error) {
	return fisc.NFCeInutilizationRemoteResult{RequestID: requestID}, nil
}

type fakeCancellationBuilder struct{}

func (fakeCancellationBuilder) BuildUnsignedCancellationEvent(
	draft fisc.NFCeCancellationDraft,
) ([]byte, string, error) {
	eventID := "ID" + fisc.NFCeCancellationEventType + draft.AccessKey + "01"
	return []byte("<evento><infEvento Id=\"" + eventID + "\"/></evento>"), eventID, nil
}

type fakeCancellationSigner struct{}

func (fakeCancellationSigner) SignCancellation(
	_ context.Context,
	_ string,
	_ string,
	unsignedXML []byte,
) ([]byte, error) {
	return append([]byte(nil), unsignedXML...), nil
}

type fakeCancellationValidator struct{}

func (fakeCancellationValidator) Validate(context.Context, []byte) error { return nil }

type fakeCancellationClient struct {
	calls int
	out   fisc.NFCeCancellationRemoteResult
}

func (f *fakeCancellationClient) Cancel(
	_ context.Context,
	_, _, _, _ string,
	_ int64,
	_ string,
	_ int,
	_ []byte,
) (fisc.NFCeCancellationRemoteResult, error) {
	f.calls++
	return f.out, nil
}

func TestNFCeReservation_IsAtomicAndIdempotentPerSale(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	var tenantID, actorUserID string
	if err := pool.QueryRow(ctx, `
		SELECT utr.tenant_id::text, utr.user_id::text
		FROM user_tenant_roles utr
		JOIN roles r ON r.id=utr.role_id AND r.name='admin'
		ORDER BY utr.user_id
		LIMIT 1
	`).Scan(&tenantID, &actorUserID); err != nil {
		t.Fatalf("seeded tenant/admin required: %v", err)
	}

	var registerID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_registers(tenant_id, name, active)
		VALUES ($1, $2, true)
		RETURNING id::text
	`, tenantID, "NFCe Reservation Integration "+time.Now().Format("150405.000000000")).Scan(&registerID); err != nil {
		t.Fatalf("create isolated cash register: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE companies
		SET ie='110042490114',
		    crt='4',
		    address_street='Avenida Fiscal',
		    address_number='100',
		    address_neighborhood='Centro',
		    address_city='Uberlandia',
		    address_city_code='3170206',
		    address_state='MG',
		    address_zip='38400000'
		WHERE id=$1
	`, tenantID); err != nil {
		t.Fatalf("prepare issuer: %v", err)
	}
	var productID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO products(
			tenant_id, sku, barcode, name, unit, cost_price, price_cash, min_stock,
			active, ncm, cest
		)
		VALUES ($1,$2,NULL,'Produto NFC-e Integration','UN',5,10,0,true,'61091000',NULL)
		RETURNING id::text
	`,
		tenantID,
		"NFCe-INT-"+time.Now().Format("150405.000000000"),
	).Scan(&productID); err != nil {
		t.Fatalf("create isolated fiscal product: %v", err)
	}

	uow := db.NewPgxUnitOfWork(pool)
	fiscalRepo := fiscinfra.NewFiscalRepo(pool)
	cscID := "1"
	cscRef := "secret://integration/nfce/csc"
	certRef := "secret://integration/nfce/certificate"
	tx, err := uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin config tx: %v", err)
	}
	if err := fiscalRepo.UpsertNFCeConfig(ctx, tx, tenantID, actorUserID, fisc.NFCeConfig{
		TenantID:             tenantID,
		Environment:          "homologation",
		Series:               321,
		CertificateSecretRef: &certRef,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("prepare NFC-e config: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit NFC-e config: %v", err)
	}

	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin fiscal profile tx: %v", err)
	}
	if err := fiscalRepo.UpsertProductFiscalProfile(ctx, tx, tenantID, actorUserID, fisc.ProductFiscalProfile{
		TenantID:         tenantID,
		ProductID:        productID,
		CFOP:             "5102",
		ICMSOrigin:       "0",
		ICMSRegime:       "csosn",
		ICMSCode:         "102",
		PISCST:           "49",
		COFINSCST:        "49",
		ReferenceVersion: "nfe-010e-v1.02|rtc-2026",
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("prepare product fiscal profile: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit product fiscal profile: %v", err)
	}

	var cashSessionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_sessions(
			tenant_id, cash_register_id, opened_by_user_id, opening_amount, status
		)
		VALUES ($1,$2,$3,0,'open')
		RETURNING id::text
	`, tenantID, registerID, actorUserID).Scan(&cashSessionID); err != nil {
		t.Fatalf("create cash session: %v", err)
	}

	var saleID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO sales(
			tenant_id, cash_session_id, status, subtotal, discount_value, total,
			profit_estimated, created_by_user_id
		)
		VALUES ($1,$2,'finalized',10,0,10,5,$3)
		RETURNING id::text
	`, tenantID, cashSessionID, actorUserID).Scan(&saleID); err != nil {
		t.Fatalf("create finalized sale: %v", err)
	}

	var saleItemID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO sale_items(
			tenant_id, sale_id, product_id, qty, unit_price, discount_value,
			subtotal, cost_unit
		)
		VALUES ($1,$2,$3,1,10,0,10,5)
		RETURNING id::text
	`, tenantID, saleID, productID).Scan(&saleItemID); err != nil {
		t.Fatalf("create sale item: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE tenant_id=$1 AND action='fiscal.nfce.reserve' AND resource_id IN (SELECT id FROM invoices WHERE tenant_id=$1 AND sale_id=$2)`, tenantID, saleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM invoices WHERE tenant_id=$1 AND sale_id=$2`, tenantID, saleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM sales WHERE tenant_id=$1 AND id=$2`, tenantID, saleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM product_fiscal_profiles WHERE tenant_id=$1 AND product_id=$2`, tenantID, productID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM products WHERE tenant_id=$1 AND id=$2`, tenantID, productID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM cash_sessions WHERE tenant_id=$1 AND id=$2`, tenantID, cashSessionID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM cash_registers WHERE tenant_id=$1 AND id=$2`, tenantID, registerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM fiscal_document_sequences WHERE tenant_id=$1 AND model=65 AND series=321`, tenantID)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := fiscapp.NewFiscalService(
		uow,
		fiscalRepo,
		salesinfra.NewSalesRepo(pool),
		invinfra.NewProductsRepo(pool),
		audit.New(pool, logger),
		validator.New(),
		logger,
	)

	issuedAt := time.Date(2026, time.September, 30, 10, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	reservation, created, err := service.ReserveNFCeDraft(
		ctx, tenantID, actorUserID, saleID, issuedAt,
	)
	if err != nil {
		t.Fatalf("ReserveNFCeDraft: %v", err)
	}
	if !created {
		t.Fatal("first reservation must be created")
	}
	if reservation.Status != "reserved" || reservation.Model != 65 || reservation.Series != 321 {
		t.Fatalf("unexpected reservation: %+v", reservation)
	}
	if reservation.DocumentNumber != 1 {
		t.Fatalf("document number=%d, want 1", reservation.DocumentNumber)
	}
	if reservation.IssuedAt != issuedAt {
		t.Fatalf("issuedAt=%s, want %s", reservation.IssuedAt, issuedAt)
	}
	if err := fisc.ValidateNFCeAccessKey(reservation.AccessKey); err != nil {
		t.Fatalf("reserved access key invalid: %v", err)
	}
	if len(reservation.NumericCode) != 8 {
		t.Fatalf("numeric code=%q, want 8 digits", reservation.NumericCode)
	}

	// Issuer identity and fiscal numbering cannot change underneath an
	// unresolved NFC-e. Certificate references may still be rotated with
	// the environment and series unchanged for recovery.
	if _, err := service.PrepareNFCeIssuerProfile(
		ctx, tenantID, actorUserID, fiscapp.PrepareNFCeIssuerRequest{
			IE: "110042490114", CRT: "4",
			AddressStreet: "Avenida Fiscal", AddressNumber: "200",
			AddressNeighborhood: "Centro", AddressCity: "Uberlandia",
			AddressCityCode: "3170206", AddressState: "MG",
			AddressZIP: "38400000",
		},
	); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("issuer mutation with reserved NFC-e must fail closed: %v", err)
	}
	var issuerNumber string
	if err := pool.QueryRow(ctx, `
		SELECT address_number FROM companies WHERE id=$1
	`, tenantID).Scan(&issuerNumber); err != nil || issuerNumber != "100" {
		t.Fatalf("issuer unexpectedly changed during reserved NFC-e: number=%q err=%v", issuerNumber, err)
	}
	if _, err := service.PrepareNFCeConfig(
		ctx, tenantID, actorUserID, fiscapp.PrepareNFCeConfigRequest{
			Environment: "production", Series: 322,
			CertificateSecretRef: certRef,
		},
	); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("environment/series change with reserved NFC-e must fail closed: %v", err)
	}
	unchangedCfg, err := fiscalRepo.GetNFCeConfig(ctx, tenantID)
	if err != nil || unchangedCfg.Environment != "homologation" || unchangedCfg.Series != 321 {
		t.Fatalf("config unexpectedly changed under reserved NFC-e: %+v err=%v", unchangedCfg, err)
	}
	rotatedRef := "secret://integration/nfce/certificate-rotated"
	rotatedCfg, err := service.PrepareNFCeConfig(
		ctx, tenantID, actorUserID, fiscapp.PrepareNFCeConfigRequest{
			Environment: "homologation", Series: 321,
			CertificateSecretRef: rotatedRef,
		},
	)
	if err != nil ||
		rotatedCfg.Environment != "homologation" ||
		rotatedCfg.Series != 321 ||
		rotatedCfg.CertificateSecretRef == nil ||
		*rotatedCfg.CertificateSecretRef != rotatedRef {
		t.Fatalf("same-series certificate rotation should remain possible: %+v err=%v", rotatedCfg, err)
	}
	restoredCfg, err := service.PrepareNFCeConfig(
		ctx, tenantID, actorUserID, fiscapp.PrepareNFCeConfigRequest{
			Environment: "homologation", Series: 321,
			CertificateSecretRef: certRef,
		},
	)
	if err != nil || restoredCfg.CertificateSecretRef == nil ||
		*restoredCfg.CertificateSecretRef != certRef {
		t.Fatalf("restore original certificate reference: %+v err=%v", restoredCfg, err)
	}

	var (
		snapshotNCM   string
		snapshotCFOP  string
		snapshotCSOSN string
		snapshotHash  string
	)
	if err := pool.QueryRow(ctx, `
		SELECT ncm, cfop, icms_code, snapshot_sha256
		FROM sale_item_fiscal_snapshots
		WHERE tenant_id=$1 AND sale_item_id=$2
	`, tenantID, saleItemID).Scan(
		&snapshotNCM, &snapshotCFOP, &snapshotCSOSN, &snapshotHash,
	); err != nil {
		t.Fatalf("read fiscal snapshot: %v", err)
	}
	if snapshotNCM != "61091000" || snapshotCFOP != "5102" || snapshotCSOSN != "102" || len(snapshotHash) != 64 {
		t.Fatalf("unexpected fiscal snapshot: ncm=%s cfop=%s csosn=%s hash=%s", snapshotNCM, snapshotCFOP, snapshotCSOSN, snapshotHash)
	}

	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin post-reservation config tx: %v", err)
	}
	if err := fiscalRepo.UpsertNFCeConfig(ctx, tx, tenantID, actorUserID, fisc.NFCeConfig{
		TenantID:             tenantID,
		Environment:          "production",
		Series:               322,
		CSCID:                &cscID,
		CSCSecretRef:         &cscRef,
		CertificateSecretRef: &certRef,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("change config after reservation: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit post-reservation config: %v", err)
	}

	if _, err := service.SetNFCeProductionTransmission(
		ctx, tenantID, actorUserID, true,
	); !errors.Is(err, common.ErrFiscalNotReady) {
		t.Fatalf("production transmission enabled without complete SEFAZ stack: %v", err)
	}

	service.SetNFCeDocumentBuilder(fakeNFCeDocumentBuilder{})
	service.SetNFCeXMLSigner(fakeNFCeSigner{})
	service.SetNFCeSchemaValidator(fakeCancellationValidator{})
	remoteStack := &fakeRemoteAuthorizer{}
	service.SetNFCeRemoteAuthorizer(remoteStack)
	service.SetNFCeCancellationBuilder(fakeCancellationBuilder{})
	service.SetNFCeCancellationSigner(fakeCancellationSigner{})
	service.SetNFCeEventSchemaValidator(fakeCancellationValidator{})
	service.SetNFCeRemoteCancellationClient(&fakeCancellationClient{})
	service.SetNFCeInutilizationBuilder(fakeNFCeDocumentBuilder{})
	service.SetNFCeInutilizationSigner(fakeNFCeSigner{})
	service.SetNFCeInutilizationSchemaValidator(fakeCancellationValidator{})
	service.SetNFCeRemoteInutilizationClient(remoteStack)

	// A fully wired provider still must not enable production while the
	// existing active seed item lacks its fiscal profile.
	if _, err := service.SetNFCeProductionTransmission(
		ctx, tenantID, actorUserID, true,
	); !errors.Is(err, common.ErrFiscalNotReady) {
		t.Fatalf("active product without fiscal profile must block production: %v", err)
	}
	var seedProductID string
	if err := pool.QueryRow(ctx, `
		SELECT id::text FROM products
		WHERE tenant_id=$1 AND sku='SKU-COCA-2L' AND active=true
	`, tenantID).Scan(&seedProductID); err != nil {
		t.Fatalf("seeded active product required for fiscal readiness: %v", err)
	}
	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin seeded fiscal profile tx: %v", err)
	}
	if err := fiscalRepo.UpsertProductFiscalProfile(ctx, tx, tenantID, actorUserID, fisc.ProductFiscalProfile{
		TenantID:         tenantID,
		ProductID:        seedProductID,
		CFOP:             "5102",
		ICMSOrigin:       "0",
		ICMSRegime:       "csosn",
		ICMSCode:         "102",
		PISCST:           "49",
		COFINSCST:        "49",
		ReferenceVersion: "nfe-010e-v1.02|rtc-2026",
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("prepare seeded fiscal profile: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit seeded fiscal profile: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM product_fiscal_profiles
			WHERE tenant_id=$1 AND product_id=$2
		`, tenantID, seedProductID)
	})
	productionCfg, err := service.SetNFCeProductionTransmission(
		ctx, tenantID, actorUserID, true,
	)
	if err != nil {
		t.Fatalf("enable production transmission with complete stack: %v", err)
	}
	if !productionCfg.Enabled || productionCfg.Environment != "production" {
		t.Fatalf("unexpected enabled production config: %+v", productionCfg)
	}

	var productionSaleID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO sales(
			tenant_id, cash_session_id, status, subtotal, discount_value, total,
			profit_estimated, created_by_user_id
		)
		VALUES ($1,$2,'finalized',10,0,10,5,$3)
		RETURNING id::text
	`, tenantID, cashSessionID, actorUserID).Scan(&productionSaleID); err != nil {
		t.Fatalf("create production-gate sale: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sale_items(
			tenant_id, sale_id, product_id, qty, unit_price, discount_value,
			subtotal, cost_unit
		)
		VALUES ($1,$2,$3,1,10,0,10,5)
	`, tenantID, productionSaleID, productID); err != nil {
		t.Fatalf("create production-gate sale item: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM audit_logs WHERE tenant_id=$1 AND resource_id IN (SELECT id FROM invoices WHERE tenant_id=$1 AND sale_id=$2)`,
			tenantID, productionSaleID,
		)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM invoices WHERE tenant_id=$1 AND sale_id=$2`,
			tenantID, productionSaleID,
		)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM sales WHERE tenant_id=$1 AND id=$2`,
			tenantID, productionSaleID,
		)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM fiscal_document_sequences WHERE tenant_id=$1 AND model=65 AND series=322`,
			tenantID,
		)
	})

	productionReservation, created, err := service.ReserveNFCeDraft(
		ctx, tenantID, actorUserID, productionSaleID, issuedAt.Add(2*time.Hour),
	)
	if err != nil {
		t.Fatalf("reserve production NFC-e behind explicit gate: %v", err)
	}
	if !created ||
		productionReservation.Environment != "production" ||
		productionReservation.Series != 322 {
		t.Fatalf("unexpected production reservation: %+v created=%t", productionReservation, created)
	}

	var contingencySaleID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO sales(
			tenant_id, cash_session_id, status, subtotal, discount_value, total,
			profit_estimated, created_by_user_id
		)
		VALUES ($1,$2,'finalized',10,0,10,5,$3)
		RETURNING id::text
	`, tenantID, cashSessionID, actorUserID).Scan(&contingencySaleID); err != nil {
		t.Fatalf("create contingency sale: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sale_items(
			tenant_id, sale_id, product_id, qty, unit_price, discount_value,
			subtotal, cost_unit
		)
		VALUES ($1,$2,$3,1,10,0,10,5)
	`, tenantID, contingencySaleID, productID); err != nil {
		t.Fatalf("create contingency sale item: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM audit_logs WHERE tenant_id=$1 AND resource_id IN (SELECT id FROM invoices WHERE tenant_id=$1 AND sale_id=$2)`,
			tenantID, contingencySaleID,
		)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM invoices WHERE tenant_id=$1 AND sale_id=$2`,
			tenantID, contingencySaleID,
		)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM sales WHERE tenant_id=$1 AND id=$2`,
			tenantID, contingencySaleID,
		)
	})

	contingencyIssuedAt := issuedAt.Add(3 * time.Hour)
	contingencyStartedAt := contingencyIssuedAt.Add(-5 * time.Minute)
	const contingencyReason = "Indisponibilidade de comunicacao com a SEFAZ durante a venda."
	contingencyReservation, created, err := service.ReserveNFCeOfflineContingency(
		ctx,
		tenantID,
		actorUserID,
		contingencySaleID,
		contingencyIssuedAt,
		contingencyStartedAt,
		contingencyReason,
	)
	if err != nil {
		t.Fatalf("reserve production offline contingency: %v", err)
	}
	if !created ||
		contingencyReservation.EmissionType != fisc.NFCeOfflineContingencyEmissionType ||
		contingencyReservation.DocumentNumber != productionReservation.DocumentNumber+1 ||
		contingencyReservation.AccessKey[34] != '9' ||
		contingencyReservation.ContingencyStartedAt == nil ||
		!contingencyReservation.ContingencyStartedAt.Equal(contingencyStartedAt) ||
		contingencyReservation.ContingencyJustification == nil ||
		*contingencyReservation.ContingencyJustification != contingencyReason {
		t.Fatalf("unexpected contingency reservation: %+v", contingencyReservation)
	}

	var (
		storedEmissionType int
		storedStartedAt    time.Time
		storedReason       string
	)
	if err := pool.QueryRow(ctx, `
		SELECT emission_type, contingency_started_at, contingency_reason
		FROM invoices
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, contingencyReservation.InvoiceID).Scan(
		&storedEmissionType,
		&storedStartedAt,
		&storedReason,
	); err != nil {
		t.Fatalf("read contingency metadata: %v", err)
	}
	if storedEmissionType != fisc.NFCeOfflineContingencyEmissionType ||
		!storedStartedAt.Equal(contingencyStartedAt) ||
		storedReason != contingencyReason {
		t.Fatalf(
			"unexpected stored contingency metadata: type=%d at=%s reason=%q",
			storedEmissionType,
			storedStartedAt,
			storedReason,
		)
	}
	if _, _, err := service.ReserveNFCeDraft(
		ctx, tenantID, actorUserID, contingencySaleID, contingencyIssuedAt,
	); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("normal reservation must not replace contingency reservation: %v", err)
	}

	productionCfg, err = service.SetNFCeProductionTransmission(
		ctx, tenantID, actorUserID, false,
	)
	if err != nil {
		t.Fatalf("disable production transmission: %v", err)
	}
	if productionCfg.Enabled {
		t.Fatalf("production transmission kill switch remained enabled: %+v", productionCfg)
	}

	replay, created, err := service.ReserveNFCeDraft(
		ctx, tenantID, actorUserID, saleID, issuedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("ReserveNFCeDraft replay: %v", err)
	}
	if created {
		t.Fatal("same sale must reuse the existing reservation")
	}
	if replay.InvoiceID != reservation.InvoiceID ||
		replay.DocumentNumber != reservation.DocumentNumber ||
		replay.AccessKey != reservation.AccessKey ||
		!replay.IssuedAt.Equal(reservation.IssuedAt) {
		t.Fatalf("replay changed reservation: first=%+v replay=%+v", reservation, replay)
	}

	var invoiceCount, auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM invoices
		WHERE tenant_id=$1 AND sale_id=$2 AND status='reserved'
	`, tenantID, saleID).Scan(&invoiceCount); err != nil {
		t.Fatalf("count reserved invoices: %v", err)
	}
	if invoiceCount != 1 {
		t.Fatalf("reserved invoice count=%d, want 1", invoiceCount)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE tenant_id=$1
		  AND action='fiscal.nfce.reserve'
		  AND resource_id=(
		    SELECT id FROM invoices WHERE tenant_id=$1 AND sale_id=$2
		  )
	`, tenantID, saleID).Scan(&auditCount); err != nil {
		t.Fatalf("count reservation audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("reservation audit count=%d, want 1", auditCount)
	}

	var nextNumber int64
	if err := pool.QueryRow(ctx, `
		SELECT next_number
		FROM fiscal_document_sequences
		WHERE tenant_id=$1 AND model=65 AND series=321
	`, tenantID).Scan(&nextNumber); err != nil {
		t.Fatalf("read sequence after reservation: %v", err)
	}
	if nextNumber != 2 {
		t.Fatalf("next number=%d, want 2 after one committed reservation", nextNumber)
	}

	calculation, err := service.PrepareLegacyOnlyTaxCalculation(
		ctx,
		tenantID,
		actorUserID,
		reservation.InvoiceID,
		saleItemID,
		"legacy-ci-v1",
	)
	if err != nil {
		t.Fatalf("PrepareLegacyOnlyTaxCalculation: %v", err)
	}
	if calculation.CalculationSHA256 == "" {
		t.Fatal("tax calculation hash is empty")
	}

	xmlID, err := service.StoreSignedNFCeXML(
		ctx, tenantID, actorUserID, reservation.InvoiceID, reservation.AccessKey,
		"NFCe-"+reservation.AccessKey+".xml", []byte("<NFe/>"),
	)
	if err != nil {
		t.Fatalf("StoreSignedNFCeXML: %v", err)
	}
	if xmlID == "" {
		t.Fatal("signed XML id is empty")
	}
	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin homologation config tx: %v", err)
	}
	if err := fiscalRepo.UpsertNFCeConfig(ctx, tx, tenantID, actorUserID, fisc.NFCeConfig{
		TenantID:             tenantID,
		Environment:          "homologation",
		Series:               321,
		CertificateSecretRef: &certRef,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("restore homologation config: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit homologation config: %v", err)
	}

	authorizedAt := time.Date(2026, time.September, 30, 10, 31, 0, 0, time.FixedZone("BRT", -3*60*60))
	remote := &fakeRemoteAuthorizer{
		authorizeOut: fisc.NFCeRemoteOutcome{
			AccessKey:  reservation.AccessKey,
			StatusCode: 103,
			Reason:     "Lote recebido com sucesso",
		},
		consultOut: fisc.NFCeRemoteOutcome{
			AccessKey:   reservation.AccessKey,
			StatusCode:  100,
			Reason:      "Autorizado o uso da NF-e",
			FinalStatus: fisc.NFCeStatusAuthorized,
			Protocol:    "131260000000001",
			ReceivedAt:  authorizedAt,
		},
	}
	service.SetNFCeRemoteAuthorizer(remote)

	firstOutcome, err := service.AuthorizeNFCeHomologation(
		ctx, tenantID, actorUserID, reservation.InvoiceID,
	)
	if err != nil {
		t.Fatalf("AuthorizeNFCeHomologation first call: %v", err)
	}
	if !firstOutcome.Pending() || remote.authorizeCalls != 1 || remote.consultCalls != 0 {
		t.Fatalf("unexpected first authorization outcome/calls: outcome=%+v authorize=%d consult=%d", firstOutcome, remote.authorizeCalls, remote.consultCalls)
	}

	var submittedStatus string
	if err := pool.QueryRow(ctx,
		"SELECT status FROM invoices WHERE tenant_id=$1 AND id=$2",
		tenantID, reservation.InvoiceID,
	).Scan(&submittedStatus); err != nil {
		t.Fatalf("read submitted invoice: %v", err)
	}
	if submittedStatus != fisc.NFCeStatusSubmitted {
		t.Fatalf("status=%s, want submitted after ambiguous authorization", submittedStatus)
	}

	secondOutcome, err := service.AuthorizeNFCeHomologation(
		ctx, tenantID, actorUserID, reservation.InvoiceID,
	)
	if err != nil {
		t.Fatalf("AuthorizeNFCeHomologation recovery: %v", err)
	}
	if !secondOutcome.Authorized() || remote.authorizeCalls != 1 || remote.consultCalls != 1 {
		t.Fatalf("recovery must consult without retransmission: outcome=%+v authorize=%d consult=%d", secondOutcome, remote.authorizeCalls, remote.consultCalls)
	}

	var status, protocol string
	var storedAuthorizedAt time.Time
	if err := pool.QueryRow(ctx,
		"SELECT status, authorization_protocol, authorized_at FROM invoices WHERE tenant_id=$1 AND id=$2",
		tenantID, reservation.InvoiceID,
	).Scan(&status, &protocol, &storedAuthorizedAt); err != nil {
		t.Fatalf("read authorized invoice: %v", err)
	}
	if status != "authorized" || protocol != secondOutcome.Protocol || !storedAuthorizedAt.Equal(authorizedAt) {
		t.Fatalf("unexpected authorized invoice state: %s %s %s", status, protocol, storedAuthorizedAt)
	}

	_, err = service.AuthorizeNFCeHomologation(
		ctx, tenantID, actorUserID, reservation.InvoiceID,
	)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("final authorization replay error=%v, want ErrConflict", err)
	}

	cancelledAt := authorizedAt.Add(time.Minute)
	cancelClient := &fakeCancellationClient{out: fisc.NFCeCancellationRemoteResult{
		AccessKey:    reservation.AccessKey,
		EventID:      "ID" + fisc.NFCeCancellationEventType + reservation.AccessKey + "01",
		Sequence:     1,
		StatusCode:   135,
		Reason:       "Evento registrado e vinculado a NF-e",
		FinalStatus:  fisc.NFCeEventStatusRegistered,
		Protocol:     "131260000000002",
		RegisteredAt: cancelledAt,
		ResponseXML:  []byte("<retEnvEvento><cStat>128</cStat></retEnvEvento>"),
	}}
	service.SetNFCeCancellationBuilder(fakeCancellationBuilder{})
	service.SetNFCeCancellationSigner(fakeCancellationSigner{})
	service.SetNFCeEventSchemaValidator(fakeCancellationValidator{})
	service.SetNFCeRemoteCancellationClient(cancelClient)

	cancelOutcome, err := service.CancelNFCeHomologation(
		ctx,
		tenantID,
		actorUserID,
		reservation.InvoiceID,
		fiscapp.CancelNFCeRequest{
			Justification: "Cancelamento de homologacao por erro operacional.",
		},
	)
	if err != nil {
		t.Fatalf("CancelNFCeHomologation: %v", err)
	}
	if !cancelOutcome.Registered() || cancelClient.calls != 1 {
		t.Fatalf("unexpected cancellation result/calls: outcome=%+v calls=%d", cancelOutcome, cancelClient.calls)
	}

	var invoiceStatus, cancellationProtocol, cancellationReason string
	var storedCancelledAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT status, cancellation_protocol, cancelled_at, cancellation_reason
		FROM invoices
		WHERE tenant_id=$1 AND id=$2
	`, tenantID, reservation.InvoiceID).Scan(
		&invoiceStatus,
		&cancellationProtocol,
		&storedCancelledAt,
		&cancellationReason,
	); err != nil {
		t.Fatalf("read cancelled invoice: %v", err)
	}
	if invoiceStatus != fisc.NFCeStatusCancelled ||
		cancellationProtocol != cancelOutcome.Protocol ||
		!storedCancelledAt.Equal(cancelledAt) ||
		cancellationReason == "" {
		t.Fatalf(
			"unexpected cancelled invoice: status=%s protocol=%s at=%s reason=%q",
			invoiceStatus,
			cancellationProtocol,
			storedCancelledAt,
			cancellationReason,
		)
	}

	var saleStatus string
	if err := pool.QueryRow(ctx,
		"SELECT status FROM sales WHERE tenant_id=$1 AND id=$2",
		tenantID, reservation.SaleID,
	).Scan(&saleStatus); err != nil {
		t.Fatalf("read sale after fiscal cancellation: %v", err)
	}
	if saleStatus != "finalized" {
		t.Fatalf("sale status=%s, want finalized after fiscal-only cancellation", saleStatus)
	}

	var eventStatus, eventProtocol string
	if err := pool.QueryRow(ctx, `
		SELECT status, protocol
		FROM invoice_fiscal_events
		WHERE tenant_id=$1 AND invoice_id=$2 AND event_type='110111'
	`, tenantID, reservation.InvoiceID).Scan(&eventStatus, &eventProtocol); err != nil {
		t.Fatalf("read cancellation event: %v", err)
	}
	if eventStatus != fisc.NFCeEventStatusRegistered || eventProtocol != cancelOutcome.Protocol {
		t.Fatalf("unexpected cancellation event: status=%s protocol=%s", eventStatus, eventProtocol)
	}
}
