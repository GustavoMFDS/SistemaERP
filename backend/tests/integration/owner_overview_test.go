//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	fininfra "github.com/example/sistemaemgo/internal/modules/finance/infrastructure"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Owner-facing metrics must aggregate the entire selected tenant, not the
// limited product list or another family member's separately owned store.
func TestOwnerOverviewAndGlobalLowStockAreTenantScoped(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	var tenantA, tenantB string
	cnpjA := fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000)
	cnpjB := fmt.Sprintf("%014d", (time.Now().UnixNano()+17)%100000000000000)
	for i, cnpj := range []string{cnpjA, cnpjB} {
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO companies(legal_name, trade_name, cnpj)
			VALUES ('Overview Integration', 'Overview Integration', $1)
			RETURNING id::text
		`, cnpj).Scan(&id); err != nil {
			t.Fatalf("create tenant %d: %v", i, err)
		}
		if i == 0 {
			tenantA = id
		} else {
			tenantB = id
		}
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM ledger_entries WHERE tenant_id=$1`, id)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM products WHERE tenant_id=$1`, id)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM companies WHERE id=$1`, id)
		})
	}
	for _, entry := range []struct {
		kind    string
		net     string
		profit  string
	}{
		{"sale", "100.00", "25.00"},
		{"sale_cancel", "-20.00", "-5.00"},
		{"return_refund", "-12.50", "0.00"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO ledger_entries(tenant_id, entry_type, amount_net, profit_estimated)
			VALUES ($1,$2,$3,$4)
		`, tenantA, entry.kind, entry.net, entry.profit); err != nil {
			t.Fatalf("seed ledger: %v", err)
		}
	}
	suffix := time.Now().UnixNano()
	for _, fixture := range []struct{ tenant string; n int }{{tenantA, 2}, {tenantB, 1}} {
		for i := 0; i < fixture.n; i++ {
			if _, err := pool.Exec(ctx, `
				INSERT INTO products(tenant_id, sku, name, unit, price_cash, min_stock, active)
				VALUES($1, $2, 'Owner Overview Stock', 'UN', 10, 5, true)
			`, fixture.tenant, fmt.Sprintf("OWNER-%d-%d", suffix, i)); err != nil {
				t.Fatalf("seed product: %v", err)
			}
		}
	}
	finance := fininfra.NewFinanceRepo(pool)
	periodA, err := finance.OwnerOverview(ctx, tenantA, "2026-01-01", "2099-12-31")
	if err != nil {
		t.Fatal(err)
	}
	if periodA.SalesAfterCancellations.String() != "80.00" ||
		periodA.EstimatedGrossProfit.String() != "20.00" ||
		periodA.RefundsRecorded.String() != "12.50" ||
		periodA.SalesCount != 1 || periodA.CancelledCount != 1 {
		t.Fatalf("wrong owner overview: %+v", periodA)
	}
	periodB, err := finance.OwnerOverview(ctx, tenantB, "2026-01-01", "2099-12-31")
	if err != nil {
		t.Fatal(err)
	}
	if periodB.SalesCount != 0 || periodB.SalesAfterCancellations != 0 || periodB.RefundsRecorded != 0 {
		t.Fatalf("tenant B leaked tenant A values: %+v", periodB)
	}
	inventory := invinfra.NewInventoryRepo(pool)
	countA, err := inventory.LowStockCount(ctx, tenantA)
	if err != nil {
		t.Fatal(err)
	}
	countB, err := inventory.LowStockCount(ctx, tenantB)
	if err != nil {
		t.Fatal(err)
	}
	if countA != 2 || countB != 1 {
		t.Fatalf("low stock tenant isolation broken: A=%d B=%d", countA, countB)
	}
	limited, err := inventory.LowStock(ctx, tenantA, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || countA != 2 {
		t.Fatalf("global count should exceed limited list: count=%d items=%d", countA, len(limited))
	}
}
