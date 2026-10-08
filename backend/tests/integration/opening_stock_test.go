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

	"github.com/example/sistemaemgo/internal/config"
	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/common"
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOpeningStockBatchIsAtomicReplaySafeAndTenantScoped(t *testing.T) {
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
	var actorID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM users WHERE active=true LIMIT 1`).Scan(&actorID); err != nil {
		t.Fatalf("seed user required: %v", err)
	}
	var tenantA, tenantB string
	now := time.Now().UnixNano()
	for i := 0; i < 2; i++ {
		cnpj := fmt.Sprintf("%014d", (now+int64(i))%100000000000000)
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO companies(legal_name, cnpj) VALUES('Opening Stock Test', $1)
			RETURNING id::text
		`, cnpj).Scan(&id); err != nil {
			t.Fatalf("seed company %d: %v", i, err)
		}
		if i == 0 {
			tenantA = id
		} else {
			tenantB = id
		}
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, tenant := range []string{tenantA, tenantB} {
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM inventory_movements WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM inventory_balances WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM opening_stock_batches WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM products WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM companies WHERE id=$1`, tenant)
		}
	}()
	skuA := fmt.Sprintf("OPENING-A-%d", now)
	skuB := fmt.Sprintf("OPENING-B-%d", now)
	for _, fixture := range []struct { tenant, sku string }{
		{tenantA, skuA}, {tenantA, skuB}, {tenantB, skuA},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO products(tenant_id, sku, name, unit, price_cash)
			VALUES ($1,$2,'Opening Test','un',15)
		`, fixture.tenant, fixture.sku); err != nil {
			t.Fatalf("seed product %s: %v", fixture.sku, err)
		}
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := invapp.NewInventoryService(
		config.Config{}, db.NewPgxUnitOfWork(pool),
		invinfra.NewInventoryRepo(pool), invinfra.NewProductsRepo(pool),
		audit.New(pool, logger), validator.New(), logger,
	)
	first := invapp.OpeningStockRequest{Items: []invapp.OpeningStockItem{
		{SKU: skuA, Quantity: platform.NewQuantityMilli(3250)},
	}}
	key := fmt.Sprintf("opening-test-%d", now)
	result, err := service.ImportOpeningStock(ctx, tenantA, actorID, key, first)
	if err != nil {
		t.Fatalf("initial opening batch: %v", err)
	}
	if result.Replayed || result.ItemCount != 1 || result.BatchID == "" {
		t.Fatalf("unexpected initial result: %+v", result)
	}
	// Read-only reconciliation returns committed batches, scoped to the tenant.
	lookedUp, found, err := service.LookupOpeningStockBatch(ctx, tenantA, key)
	if err != nil || !found || lookedUp.BatchID != result.BatchID || lookedUp.ItemCount != 1 {
		t.Fatalf("committed batch not recoverable: %+v found=%v err=%v", lookedUp, found, err)
	}
	_, found, err = service.LookupOpeningStockBatch(ctx, tenantB, key)
	if err != nil || found {
		t.Fatalf("another tenant saw A's batch: found=%v err=%v", found, err)
	}
	_, found, err = service.LookupOpeningStockBatch(ctx, tenantA, "missing-safe-key")
	if err != nil || found {
		t.Fatalf("nonexistent batch was reported as committed: found=%v err=%v", found, err)
	}
	_, _, err = service.LookupOpeningStockBatch(ctx, tenantA, "short")
	if !errors.Is(err, common.ErrValidation) {
		t.Fatalf("invalid lookup key must be rejected, got %v", err)
	}

	replayed, err := service.ImportOpeningStock(ctx, tenantA, actorID, key, first)
	if err != nil || !replayed.Replayed || replayed.BatchID != result.BatchID {
		t.Fatalf("replay must return same batch without new movement: %+v, %v", replayed, err)
	}
	changed := invapp.OpeningStockRequest{Items: []invapp.OpeningStockItem{
		{SKU: skuA, Quantity: platform.NewQuantityMilli(4000)},
	}}
	_, err = service.ImportOpeningStock(ctx, tenantA, actorID, key, changed)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("changing payload with same key must conflict, got: %v", err)
	}
	_, err = service.ImportOpeningStock(ctx, tenantA, actorID, key+"-other", first)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("new key must not duplicate already opened product, got: %v", err)
	}
	// A mixed batch cannot create stock for an eligible product if another
	// line already has a history; the whole transaction must roll back.
	mixed := invapp.OpeningStockRequest{Items: []invapp.OpeningStockItem{
		{SKU: skuB, Quantity: platform.NewQuantityMilli(6000)},
		{SKU: skuA, Quantity: platform.NewQuantityMilli(1500)},
	}}
	_, err = service.ImportOpeningStock(ctx, tenantA, actorID, key+"-mixed", mixed)
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("mixed valid/used batch must conflict: %v", err)
	}
	var qtyA, qtyB string
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(b.qty_on_hand,0)::numeric(14,3)::text
		FROM products p LEFT JOIN inventory_balances b ON p.id=b.product_id
		WHERE p.tenant_id=$1 AND p.sku=$2
	`, tenantA, skuA).Scan(&qtyA); err != nil { t.Fatal(err) }
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(b.qty_on_hand,0)::numeric(14,3)::text
		FROM products p LEFT JOIN inventory_balances b ON p.id=b.product_id
		WHERE p.tenant_id=$1 AND p.sku=$2
	`, tenantA, skuB).Scan(&qtyB); err != nil { t.Fatal(err) }
	if qtyA != "3.250" || qtyB != "0.000" {
		t.Fatalf("balances changed despite replay/rollback: %q, %q", qtyA, qtyB)
	}
	var movements int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM inventory_movements
		WHERE tenant_id=$1 AND reference_type='opening_stock'
	`, tenantA).Scan(&movements); err != nil { t.Fatal(err) }
	if movements != 1 {
		t.Fatalf("expected one committed opening movement, got %d", movements)
	}
	// Separate CNPJ/tenant: even the same product SKU and key are independent.
	other, err := service.ImportOpeningStock(ctx, tenantB, actorID, key, first)
	if err != nil || other.Replayed || other.BatchID == result.BatchID {
		t.Fatalf("other tenant must have independent opening batch: %+v, %v", other, err)
	}
}
