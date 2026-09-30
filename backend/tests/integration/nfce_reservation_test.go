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
	if _, err := pool.Exec(ctx, `
		UPDATE products
		SET ncm='61091000'
		WHERE tenant_id=$1 AND active=true
	`, tenantID); err != nil {
		t.Fatalf("prepare product NCM: %v", err)
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
		CSCID: &cscID,
		CSCSecretRef: &cscRef,
		CertificateSecretRef: &certRef,
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("prepare NFC-e config: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit NFC-e config: %v", err)
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

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_logs WHERE tenant_id=$1 AND action='fiscal.nfce.reserve' AND resource_id IN (SELECT id FROM invoices WHERE tenant_id=$1 AND sale_id=$2)`, tenantID, saleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM invoices WHERE tenant_id=$1 AND sale_id=$2`, tenantID, saleID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM sales WHERE tenant_id=$1 AND id=$2`, tenantID, saleID)
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
}
