//go:build integration

package integration_test

import (
	"context"
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
		TenantID: tenantID,
		Environment: "homologation",
		Series: 321,
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
		TenantID: tenantID,
		ProductID: productID,
		CFOP: "5102",
		ICMSOrigin: "0",
		ICMSRegime: "csosn",
		ICMSCode: "102",
		PISCST: "49",
		COFINSCST: "49",
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

	var (
		snapshotNCM string
		snapshotCFOP string
		snapshotCSOSN string
		snapshotHash string
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
		TenantID: tenantID,
		Environment: "production",
		Series: 322,
		CSCID: &cscID,
		CSCSecretRef: &cscRef,
		CertificateSecretRef: &certRef,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("change config after reservation: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit post-reservation config: %v", err)
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
	if err := service.MarkNFCeSubmitted(
		ctx, tenantID, actorUserID, reservation.InvoiceID, reservation.AccessKey,
	); err != nil {
		t.Fatalf("MarkNFCeSubmitted: %v", err)
	}

	authorizedAt := time.Date(2026, time.September, 30, 10, 31, 0, 0, time.FixedZone("BRT", -3*60*60))
	result := fisc.NFCeAuthorizationResult{
		Status: fisc.NFCeStatusAuthorized,
		AccessKey: reservation.AccessKey,
		Protocol: "131260000000001",
		AuthorizedAt: authorizedAt,
	}
	if err := service.ApplyNFCeAuthorizationResult(
		ctx, tenantID, actorUserID, reservation.InvoiceID, result,
	); err != nil {
		t.Fatalf("ApplyNFCeAuthorizationResult: %v", err)
	}

	var status, protocol string
	var storedAuthorizedAt time.Time
	if err := pool.QueryRow(ctx,
		"SELECT status, authorization_protocol, authorized_at FROM invoices WHERE tenant_id=$1 AND id=$2",
		tenantID, reservation.InvoiceID,
	).Scan(&status, &protocol, &storedAuthorizedAt); err != nil {
		t.Fatalf("read authorized invoice: %v", err)
	}
	if status != "authorized" || protocol != result.Protocol || !storedAuthorizedAt.Equal(authorizedAt) {
		t.Fatalf("unexpected authorized invoice state: %s %s %s", status, protocol, storedAuthorizedAt)
	}

	err = service.ApplyNFCeAuthorizationResult(
		ctx, tenantID, actorUserID, reservation.InvoiceID, result,
	)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("authorization replay error=%v, want ErrConflict", err)
	}
}
