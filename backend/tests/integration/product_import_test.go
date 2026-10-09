//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProductImportIsAtomicReplaySafeAndTenantScoped(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	var actor string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM users WHERE active=true LIMIT 1`).Scan(&actor); err != nil {
		t.Fatalf("seed user required: %v", err)
	}
	now := time.Now().UnixNano()
	var companies []string
	for i := 0; i < 2; i++ {
		var tenant string
		cnpj := fmt.Sprintf("%014d", (now + int64(i)) % 100000000000000)
		if err := pool.QueryRow(ctx, `
			INSERT INTO companies(legal_name, cnpj) VALUES('Product Batch Test', $1)
			RETURNING id::text
		`, cnpj).Scan(&tenant); err != nil {
			t.Fatal(err)
		}
		companies = append(companies, tenant)
		if _, err := pool.Exec(ctx, `
			INSERT INTO user_tenants(user_id, tenant_id) VALUES ($1,$2)
		`, actor, tenant); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, tenant := range companies {
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM inventory_balances WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM products WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM product_import_batches WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM user_tenant_roles WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM user_tenants WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM companies WHERE id=$1`, tenant)
		}
	}()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := invapp.NewProductsService(db.NewPgxUnitOfWork(pool), invinfra.NewProductsRepo(pool),
		audit.New(pool, logger), validator.New(), logger)
	tenantA, tenantB := companies[0], companies[1]
	sku1 := fmt.Sprintf("BATCH-%d-A", now)
	sku2 := fmt.Sprintf("BATCH-%d-B", now)
	barcode := fmt.Sprintf("%016d", now%10000000000000000)
	product := func(sku string) invapp.ProductCreateRequest {
		return invapp.ProductCreateRequest{
			SKU: sku, Name: "Produto em lote", Unit: "un",
			CostPrice: 0, PriceCash: platform.NewMoneyCents(1590),
			MinStock: platform.NewQuantityMilli(1000), Active: true,
		}
	}
	first := product(sku1)
	first.Barcode = &barcode
	second := product(sku2)
	items := invapp.ProductImportRequest{Items: []invapp.ProductCreateRequest{first, second}}
	key := fmt.Sprintf("product-batch-test-%d", now)
	receipt, err := svc.ImportProducts(ctx, tenantA, actor, key, items)
	if err != nil || receipt.Replayed || receipt.BatchID == "" || receipt.ItemCount != 2 {
		t.Fatalf("first atomic import: %+v %v", receipt, err)
	}
	recovered, found, err := svc.LookupProductImportBatch(ctx, tenantA, key)
	if err != nil || !found || recovered.BatchID != receipt.BatchID {
		t.Fatalf("committed receipt not visible: %+v found=%v err=%v", recovered, found, err)
	}
	_, found, err = svc.LookupProductImportBatch(ctx, tenantB, key)
	if err != nil || found {
		t.Fatalf("company B read A receipt: found=%v err=%v", found, err)
	}

	// Item ordering and whitespace normalization are not material differences.
	reordered := invapp.ProductImportRequest{Items: []invapp.ProductCreateRequest{second, first}}
	reordered.Items[1].Name = "  Produto em lote  "
	replay, err := svc.ImportProducts(ctx, tenantA, actor, key, reordered)
	if err != nil || !replay.Replayed || replay.BatchID != receipt.BatchID {
		t.Fatalf("idempotency failed: %+v %v", replay, err)
	}
	modified := invapp.ProductImportRequest{Items: []invapp.ProductCreateRequest{first, second}}
	modified.Items[1].PriceCash = platform.NewMoneyCents(2500)
	_, err = svc.ImportProducts(ctx, tenantA, actor, key, modified)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("same key different payload must conflict: %v", err)
	}

	// Existing SKU collides on the second row after the first row was inserted.
	// No new product, balance, receipt or audit may survive this transaction.
	third := product(fmt.Sprintf("BATCH-%d-0", now))
	collision := invapp.ProductImportRequest{Items: []invapp.ProductCreateRequest{third, first}}
	failedKey := key + "-rollback"
	_, err = svc.ImportProducts(ctx, tenantA, actor, failedKey, collision)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("SKU collision must roll back entire batch: %v", err)
	}
	_, found, err = svc.LookupProductImportBatch(ctx, tenantA, failedKey)
	if err != nil || found {
		t.Fatalf("failed transaction left a committed receipt: found=%v err=%v", found, err)
	}

	var products, balances, auditEvents int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM products WHERE tenant_id=$1`, tenantA).Scan(&products); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM inventory_balances WHERE tenant_id=$1`, tenantA).Scan(&balances); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND action='product.import.batch'
	`, tenantA).Scan(&auditEvents); err != nil {
		t.Fatal(err)
	}
	if products != 2 || balances != 2 || auditEvents != 1 {
		t.Fatalf("partial or replayed writes: products=%d balances=%d audit=%d", products, balances, auditEvents)
	}
	// A second independent company may use exactly the same key and SKUs.
	other, err := svc.ImportProducts(ctx, tenantB, actor, key, items)
	if err != nil || other.Replayed || other.BatchID == receipt.BatchID {
		t.Fatalf("tenant B import not independent: %+v %v", other, err)
	}
	// A second receipt in A proves the history is paginated and not leaking B.
	fourth := product(fmt.Sprintf("BATCH-%d-D", now))
	secondReceipt, err := svc.ImportProducts(ctx, tenantA, actor, key+"-second",
		invapp.ProductImportRequest{Items: []invapp.ProductCreateRequest{fourth}})
	if err != nil {
		t.Fatalf("second tenant A receipt: %v", err)
	}
	firstPage, err := svc.ListImportHistory(ctx, tenantA, 1, 0, invapp.ImportHistoryFilter{})
	if err != nil || !firstPage.HasMore || len(firstPage.Items) != 1 ||
		firstPage.Items[0].BatchID != secondReceipt.BatchID || firstPage.Items[0].ItemCount != 1 {
		t.Fatalf("first history page must show newest receipt: %+v err=%v", firstPage, err)
	}
	secondPage, err := svc.ListImportHistory(ctx, tenantA, 1, 1, invapp.ImportHistoryFilter{})
	if err != nil || secondPage.HasMore || len(secondPage.Items) != 1 ||
		secondPage.Items[0].BatchID != receipt.BatchID || secondPage.Items[0].ActorName == "" {
		t.Fatalf("second history page must show original receipt: %+v err=%v", secondPage, err)
	}
	otherHistory, err := svc.ListImportHistory(ctx, tenantB, 20, 0, invapp.ImportHistoryFilter{})
	if err != nil || len(otherHistory.Items) != 1 || otherHistory.Items[0].BatchID != other.BatchID {
		t.Fatalf("tenant B must see only its own history: %+v err=%v", otherHistory, err)
	}
	if _, err := svc.ListImportHistory(ctx, tenantA, 51, 0, invapp.ImportHistoryFilter{}); !errors.Is(err, common.ErrValidation) {
		t.Fatalf("limit over 50 should fail-closed: %v", err)
	}
	if _, err := svc.ListImportHistory(ctx, tenantA, 20, -1, invapp.ImportHistoryFilter{}); !errors.Is(err, common.ErrValidation) {
		t.Fatalf("negative offset should fail-closed: %v", err)
	}

	// Inclusive business-day range returns the two receipts of company A.
	businessDay := time.Now().UTC().Add(-3 * time.Hour).Format("2006-01-02")
	filtered, err := svc.ListImportHistory(ctx, tenantA, 10, 0, invapp.ImportHistoryFilter{
		From: businessDay, To: businessDay,
	})
	if err != nil || len(filtered.Items) != 2 {
		t.Fatalf("Brazilian calendar filter lost batch receipts: %+v err=%v", filtered, err)
	}
	absent, err := svc.ListImportHistory(ctx, tenantA, 10, 0, invapp.ImportHistoryFilter{
		From: "2000-01-01", To: "2000-01-02",
	})
	if err != nil || len(absent.Items) != 0 {
		t.Fatalf("old period must exclude current receipts: %+v err=%v", absent, err)
	}
	toExport, err := svc.ExportImportHistory(ctx, tenantA, invapp.ImportHistoryFilter{
		From: businessDay, To: businessDay,
	})
	if err != nil || len(toExport) != 2 {
		t.Fatalf("filter export must match company A receipts: %+v err=%v", toExport, err)
	}
	otherExport, err := svc.ExportImportHistory(ctx, tenantB, invapp.ImportHistoryFilter{})
	if err != nil || len(otherExport) != 1 || otherExport[0].BatchID != other.BatchID {
		t.Fatalf("export leaked receipt from another company: %+v err=%v", otherExport, err)
	}

	// A bad actor relationship must roll back even after row insertion.
	_, err = svc.ImportProducts(ctx, tenantA,
		"00000000-0000-4000-8000-000000000001", key+"-bad-actor",
		invapp.ProductImportRequest{Items: []invapp.ProductCreateRequest{third}})
	if err == nil {
		t.Fatal("missing actor-tenant membership was accepted")
	}
	_, found, err = svc.LookupProductImportBatch(ctx, tenantA, key+"-bad-actor")
	if err != nil || found {
		t.Fatalf("unauthorized actor left receipt: found=%v err=%v", found, err)
	}
}
