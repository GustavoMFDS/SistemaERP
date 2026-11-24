//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	fininfra "github.com/example/sistemaemgo/internal/modules/finance/infrastructure"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFinanceBusinessDayIgnoresDatabaseSessionTimezone(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var tenant, actor, register, session, sale string
	if err := pool.QueryRow(ctx, "SELECT id::text FROM users WHERE email='admin@sistema.local'").Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO companies(legal_name,trade_name,cnpj) VALUES ('Business day CI','Business day CI',$1) RETURNING id::text", fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000)).Scan(&tenant); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, query := range []string{
			"DELETE FROM ledger_entries WHERE tenant_id=$1",
			"DELETE FROM payments WHERE tenant_id=$1",
			"DELETE FROM sales WHERE tenant_id=$1",
			"DELETE FROM cash_sessions WHERE tenant_id=$1",
			"DELETE FROM cash_registers WHERE tenant_id=$1",
			"DELETE FROM user_tenants WHERE tenant_id=$1",
			"DELETE FROM companies WHERE id=$1",
		} {
			if _, err := pool.Exec(cleanup, query, tenant); err != nil {
				t.Errorf("cleanup business day fixture: %v", err)
			}
		}
	}()
	if _, err := pool.Exec(ctx, "INSERT INTO user_tenants(user_id,tenant_id) VALUES ($1,$2)", actor, tenant); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO cash_registers(tenant_id,name) VALUES ($1,'Business day CI') RETURNING id::text", tenant).Scan(&register); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,status) VALUES ($1,$2,$3,'closed') RETURNING id::text", tenant, register, actor).Scan(&session); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO sales(tenant_id,cash_session_id,created_by_user_id,subtotal,total) VALUES ($1,$2,$3,100,100) RETURNING id::text", tenant, session, actor).Scan(&sale); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ instant, amount string }{
		{"2026-11-24T02:59:59Z", "1.00"},
		{"2026-11-24T03:00:00Z", "10.00"},
		{"2026-11-25T02:59:59Z", "20.00"},
		{"2026-11-25T03:00:00Z", "40.00"},
	} {
		if _, err := pool.Exec(ctx, "INSERT INTO ledger_entries(tenant_id,entry_type,sale_id,amount_net,created_at) VALUES ($1,'sale',$2,$3,$4::timestamptz)", tenant, sale, entry.amount, entry.instant); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO payments(tenant_id,sale_id,method,amount,created_at) VALUES ($1,$2,'cash',$3,$4::timestamptz)", tenant, sale, entry.amount, entry.instant); err != nil {
			t.Fatal(err)
		}
	}
	repo := fininfra.NewFinanceRepo(pool)
	overview, err := repo.OwnerOverview(ctx, tenant, "2026-11-24", "2026-11-24")
	if err != nil || overview.SalesCount != 2 || overview.SalesAfterCancellations.String() != "30.00" {
		t.Fatalf("business day overview: %+v err=%v", overview, err)
	}
	totals, err := repo.Dashboard(ctx, tenant, "2026-11-24", "2026-11-24")
	if err != nil || totals["sale"].String() != "30.00" {
		t.Fatalf("business day dashboard: %+v err=%v", totals, err)
	}
	payments, count, err := repo.ListPayments(ctx, tenant, "2026-11-24", "2026-11-24", "", "", 100, 0)
	if err != nil || count != 2 || len(payments) != 2 {
		t.Fatalf("business day payments: count=%d rows=%d err=%v", count, len(payments), err)
	}
	for _, payment := range payments {
		if payment.Amount.String() != "10.00" && payment.Amount.String() != "20.00" {
			t.Fatalf("out-of-window payment: %s", payment.Amount.String())
		}
	}
}
