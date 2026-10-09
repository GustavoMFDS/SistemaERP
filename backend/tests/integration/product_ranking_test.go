//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	fininfra "github.com/example/sistemaemgo/internal/modules/finance/infrastructure"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Direct SQL fixtures prove the report is about finalized sale items only,
// not global product identifiers, cancelled rows, or another company.
func TestProductRankingByCNPJAndStatus(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, done := context.WithTimeout(context.Background(), 35*time.Second)
	defer done()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	var admin string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM users WHERE email='admin@sistema.local'`).Scan(&admin); err != nil {
		t.Fatal(err)
	}
	type fixture struct {
		tenant, register, session, product string
	}
	var fixtures []fixture
	defer func() {
		cleanup, end := context.WithTimeout(context.Background(), 15*time.Second)
		defer end()
		for _, f := range fixtures {
			_, _ = pool.Exec(cleanup, `DELETE FROM sale_items WHERE tenant_id=$1`, f.tenant)
			_, _ = pool.Exec(cleanup, `DELETE FROM sales WHERE tenant_id=$1`, f.tenant)
			_, _ = pool.Exec(cleanup, `DELETE FROM cash_sessions WHERE tenant_id=$1`, f.tenant)
			_, _ = pool.Exec(cleanup, `DELETE FROM cash_registers WHERE tenant_id=$1`, f.tenant)
			_, _ = pool.Exec(cleanup, `DELETE FROM products WHERE tenant_id=$1`, f.tenant)
			_, _ = pool.Exec(cleanup, `DELETE FROM user_tenants WHERE tenant_id=$1`, f.tenant)
			_, _ = pool.Exec(cleanup, `DELETE FROM companies WHERE id=$1`, f.tenant)
		}
	}()
	for i := 0; i < 2; i++ {
		var f fixture
		if err := pool.QueryRow(ctx, `
			INSERT INTO companies(legal_name,cnpj)
			VALUES('Ranking Integration', $1)
			RETURNING id::text
		`, fmt.Sprintf("%014d", (time.Now().UnixNano()+int64(i))%100000000000000)).
			Scan(&f.tenant); err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, f)
		if _, err := pool.Exec(ctx, `INSERT INTO user_tenants(user_id,tenant_id,active) VALUES($1,$2,true)`,
			admin, f.tenant); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO cash_registers(tenant_id,name,active)
			VALUES($1,'Caixa Ranking',true) RETURNING id::text
		`, f.tenant).Scan(&f.register); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,opening_amount)
			VALUES($1,$2,$3,0) RETURNING id::text
		`, f.tenant, f.register, admin).Scan(&f.session); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, `
			INSERT INTO products(tenant_id,sku,name,unit,cost_price,price_cash,min_stock,active)
			VALUES($1,'SKU-RANK','Produto Ranking','un',0,10.00,0,true)
			RETURNING id::text
		`, f.tenant).Scan(&f.product); err != nil {
			t.Fatal(err)
		}
		fixtures[i] = f
	}
	insertSale := func(f fixture, status string, qty, subtotal string) {
		t.Helper()
		var saleID string
		if err := pool.QueryRow(ctx, `
			INSERT INTO sales(tenant_id,cash_session_id,status,subtotal,discount_value,total,
				profit_estimated,created_by_user_id)
			VALUES($1,$2,$3,$4,0,$4,0,$5) RETURNING id::text
		`, f.tenant, f.session, status, subtotal, admin).Scan(&saleID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO sale_items(tenant_id,sale_id,product_id,qty,unit_price,discount_value,subtotal,cost_unit)
			VALUES($1,$2,$3,$4,10.00,0,$5,0)
		`, f.tenant, saleID, f.product, qty, subtotal); err != nil {
			t.Fatal(err)
		}
	}
	insertSale(fixtures[0], "finalized", "2.000", "20.00")
	insertSale(fixtures[0], "cancelled", "3.000", "30.00")
	insertSale(fixtures[1], "finalized", "7.000", "70.00")
	zone, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(zone).Format("2006-01-02")
	repo := fininfra.NewFinanceRepo(pool)
	rows, err := repo.ProductRanking(ctx, fixtures[0].tenant, today, today, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("tenant A ranking unexpected: %+v err=%v", rows, err)
	}
	amount, err := strconv.ParseFloat(rows[0].ItemTotal, 64)
	if err != nil || amount != 20.00 || rows[0].SalesCount != 1 ||
		rows[0].ProductID != fixtures[0].product || rows[0].SKU != "SKU-RANK" {
		t.Fatalf("tenant A includes cancelled or other-tenant items: %+v", rows[0])
	}
	b, err := repo.ProductRanking(ctx, fixtures[1].tenant, today, today, 10)
	if err != nil || len(b) != 1 {
		t.Fatalf("tenant B ranking unexpected: %+v err=%v", b, err)
	}
	bAmount, err := strconv.ParseFloat(b[0].ItemTotal, 64)
	if err != nil || bAmount != 70 || b[0].ProductID != fixtures[1].product {
		t.Fatalf("tenant B ranking leaked or miscalculated items: %+v", b[0])
	}
	empty, err := repo.ProductRanking(ctx, fixtures[0].tenant, "2001-01-01", "2001-01-02", 10)
	if err != nil || len(empty) != 0 {
		t.Fatalf("ranking date filter did not exclude recent sales: %+v err=%v", empty, err)
	}
}
