//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	fiscapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	fiscinfra "github.com/example/sistemaemgo/internal/modules/fiscal/infrastructure"
	fiscmvp "github.com/example/sistemaemgo/internal/modules/fiscal/providers/mvp"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	salesinfra "github.com/example/sistemaemgo/internal/modules/sales/infrastructure"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFiscalGenerationCommitsInvoiceXMLAndAuditTogether(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	var tenantID, userID, productID string
	if err := pool.QueryRow(ctx, `
		SELECT p.tenant_id::text, ut.user_id::text, p.id::text
		FROM products p
		JOIN user_tenants ut
		  ON ut.tenant_id=p.tenant_id
		 AND ut.active=true
		JOIN users u
		  ON u.id=ut.user_id
		 AND u.active=true
		ORDER BY p.created_at, ut.created_at
		LIMIT 1
	`).Scan(&tenantID, &userID, &productID); err != nil {
		t.Fatalf("seeded tenant/user/product required: %v", err)
	}

	suffix := time.Now().UnixNano()
	var registerID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_registers(tenant_id, name, active)
		VALUES ($1,$2,true)
		RETURNING id::text
	`, tenantID, fmt.Sprintf("Round9 Fiscal Caixa %d", suffix)).Scan(&registerID); err != nil {
		t.Fatalf("create register: %v", err)
	}

	var sessionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_sessions(
			tenant_id, cash_register_id, opened_by_user_id, opening_amount, status
		)
		VALUES ($1,$2,$3,0,'open')
		RETURNING id::text
	`, tenantID, registerID, userID).Scan(&sessionID); err != nil {
		t.Fatalf("create cash session: %v", err)
	}

	var saleID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO sales(
			tenant_id, cash_session_id, status, subtotal, discount_value,
			total, profit_estimated, created_by_user_id
		)
		VALUES ($1,$2,'finalized',10,0,10,4,$3)
		RETURNING id::text
	`, tenantID, sessionID, userID).Scan(&saleID); err != nil {
		t.Fatalf("create sale: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO sale_items(
			tenant_id, sale_id, product_id, qty, unit_price,
			discount_value, subtotal, cost_unit
		)
		VALUES ($1,$2,$3,1,10,0,10,6)
	`, tenantID, saleID, productID); err != nil {
		t.Fatalf("create sale item: %v", err)
	}

	var invoiceID, xmlID string
	t.Cleanup(func() {
		bg := context.Background()
		if xmlID != "" {
			_, _ = pool.Exec(bg, `DELETE FROM audit_logs WHERE resource_id=$1::uuid`, xmlID)
		}
		if invoiceID != "" {
			_, _ = pool.Exec(bg, `DELETE FROM invoice_xml_files WHERE invoice_id=$1`, invoiceID)
			_, _ = pool.Exec(bg, `DELETE FROM invoices WHERE id=$1`, invoiceID)
		}
		_, _ = pool.Exec(bg, `DELETE FROM sale_items WHERE sale_id=$1`, saleID)
		_, _ = pool.Exec(bg, `DELETE FROM sales WHERE id=$1`, saleID)
		_, _ = pool.Exec(bg, `DELETE FROM cash_sessions WHERE id=$1`, sessionID)
		_, _ = pool.Exec(bg, `DELETE FROM cash_registers WHERE id=$1`, registerID)
	})

	fiscalRepo := fiscinfra.NewFiscalRepo(pool)
	salesRepo := salesinfra.NewSalesRepo(pool)
	productsRepo := invinfra.NewProductsRepo(pool)
	auditSvc := audit.New(pool, slog.Default())
	svc := fiscapp.NewFiscalServiceWithProvider(
		db.NewPgxUnitOfWork(pool),
		fiscalRepo,
		salesRepo,
		productsRepo,
		fiscmvp.New(),
		auditSvc,
		validator.New(),
		slog.Default(),
	)

	invoiceID, xmlID, err = svc.GenerateNFeXML(
		ctx,
		tenantID,
		userID,
		fiscapp.GenerateXMLRequest{SaleID: saleID},
	)
	if err != nil {
		t.Fatalf("GenerateNFeXML: %v", err)
	}
	if invoiceID == "" || xmlID == "" {
		t.Fatalf("missing fiscal ids invoice=%q xml=%q", invoiceID, xmlID)
	}

	var invoices, xmls, audits int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM invoices WHERE tenant_id=$1 AND id=$2),
			(SELECT count(*) FROM invoice_xml_files WHERE tenant_id=$1 AND id=$3),
			(SELECT count(*) FROM audit_logs
			  WHERE tenant_id=$1
			    AND resource_id=$3
			    AND action='fiscal.nfe_xml.generate')
	`, tenantID, invoiceID, xmlID).Scan(&invoices, &xmls, &audits); err != nil {
		t.Fatalf("read fiscal committed evidence: %v", err)
	}
	if invoices != 1 || xmls != 1 || audits != 1 {
		t.Fatalf(
			"fiscal evidence invoices=%d xmls=%d audits=%d, want 1/1/1",
			invoices,
			xmls,
			audits,
		)
	}

	name, content, err := svc.DownloadXML(ctx, tenantID, uuid.NewString(), xmlID)
	if err == nil {
		t.Fatalf("download with invalid audit actor must fail, name=%q bytes=%d", name, len(content))
	}
	if name != "" || content != nil {
		t.Fatalf("failed audited download must release no file: name=%q content=%v", name, content)
	}

	var downloadAudits int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE tenant_id=$1
		  AND resource_id=$2
		  AND action='fiscal.nfe_xml.download'
	`, tenantID, xmlID).Scan(&downloadAudits); err != nil {
		t.Fatalf("count failed download audit: %v", err)
	}
	if downloadAudits != 0 {
		t.Fatalf("failed download left %d audit row(s)", downloadAudits)
	}

	name, content, err = svc.DownloadXML(ctx, tenantID, userID, xmlID)
	if err != nil {
		t.Fatalf("valid audited download: %v", err)
	}
	if name == "" || len(content) == 0 {
		t.Fatalf("valid audited download returned empty file: name=%q bytes=%d", name, len(content))
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE tenant_id=$1
		  AND actor_user_id=$2
		  AND resource_id=$3
		  AND action='fiscal.nfe_xml.download'
	`, tenantID, userID, xmlID).Scan(&downloadAudits); err != nil {
		t.Fatalf("count committed download audit: %v", err)
	}
	if downloadAudits != 1 {
		t.Fatalf("committed download audit count=%d, want 1", downloadAudits)
	}
}
