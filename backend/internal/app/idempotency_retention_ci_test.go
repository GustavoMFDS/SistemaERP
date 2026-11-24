package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Exercise the real startup maintenance over the four migrated tables in a
// dedicated CI database. The job owns the whole database and drops it afterwards.
func TestIdempotencyRetentionAllTables(t *testing.T) {
	dsn := os.Getenv("TEST_IDEMPOTENCY_RETENTION_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires dedicated disposable idempotency retention database")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" {
		t.Fatal("retention contract is restricted to disposable hosted CI")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("could not open retention fixture database")
	}
	defer pool.Close()
	var database string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil || database != "sistemaemgo_retention_ci" {
		t.Fatal("unexpected retention database identity")
	}

	// A single transaction fixes now() for all fixtures and makes the
	// setup atomic. Use real parent records even for polymorphic key tables.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var tenant, actor, register, session, sale, returned, otherTenant, otherRegister, otherSession, otherSale, otherReturn string
	mustID := func(dst *string, query string, args ...any) {
		t.Helper()
		if err := tx.QueryRow(ctx, query, args...).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	mustID(&actor, "SELECT id::text FROM users WHERE email='admin@sistema.local'")
	createFixture := func(cnpj string, tenant, register, session, sale, returned *string) {
		t.Helper()
		mustID(tenant, "INSERT INTO companies(legal_name,trade_name,cnpj) VALUES ('Retention CI','Retention CI',$1) RETURNING id::text", cnpj)
		if _, err := tx.Exec(ctx, "INSERT INTO user_tenants(user_id,tenant_id) VALUES ($1,$2)", actor, *tenant); err != nil {
			t.Fatal(err)
		}
		mustID(register, "INSERT INTO cash_registers(tenant_id,name) VALUES ($1,'Retention CI') RETURNING id::text", *tenant)
		mustID(session, "INSERT INTO cash_sessions(tenant_id,cash_register_id,opened_by_user_id,status) VALUES ($1,$2,$3,'closed') RETURNING id::text", *tenant, *register, actor)
		mustID(sale, "INSERT INTO sales(tenant_id,cash_session_id,created_by_user_id,subtotal,total) VALUES ($1,$2,$3,10,10) RETURNING id::text", *tenant, *session, actor)
		mustID(returned, "INSERT INTO sale_returns(tenant_id,sale_id,reason,created_by_user_id) VALUES ($1,$2,'Retention fixture',$3) RETURNING id::text", *tenant, *sale, actor)
	}
	createFixture("00000000005401", &tenant, &register, &session, &sale, &returned)
	createFixture("00000000005402", &otherTenant, &otherRegister, &otherSession, &otherSale, &otherReturn)

	type tableFixture struct {
		name, column, resource, otherResource string
	}
	tables := []tableFixture{
		{"idempotency_keys", "sale_id", sale, otherSale},
		{"procurement_idempotency_keys", "resource_id", sale, otherSale},
		{"return_idempotency_keys", "return_id", returned, otherReturn},
		{"finance_idempotency_keys", "resource_id", sale, otherSale},
	}
	for _, table := range tables {
		columns := "tenant_id,operation,idem_key,request_hash," + table.column + ",created_at"
		values := "$1,'retention.ci',key,'fixture',$2,ts"
		if table.name == "idempotency_keys" {
			columns += ",total"
			values += ",10"
		}
		// 2001 expired records force the second batch; the recent
		// record and a second tenant with the same key must survive.
		query := "INSERT INTO " + table.name + "(" + columns + ") SELECT " + values +
			" FROM (SELECT 'old-' || i AS key, now()-interval '31 days' AS ts FROM generate_series(1,2001) i UNION ALL SELECT 'recent',now()-interval '29 days') f"
		if _, err := tx.Exec(ctx, query, tenant, table.resource); err != nil {
			t.Fatalf("seed %s: %v", table.name, err)
		}
		query = "INSERT INTO " + table.name + "(" + columns + ") SELECT " + values +
			" FROM (SELECT 'recent' AS key, now()-interval '1 day' AS ts) f"
		if _, err := tx.Exec(ctx, query, otherTenant, table.otherResource); err != nil {
			t.Fatalf("seed second tenant %s: %v", table.name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for range 2 {
		runCtx, stop := context.WithCancel(ctx)
		startIdempotencyMaintenance(runCtx, pool, logger)
		stop()
		for _, table := range tables {
			var old, recent, other int
			query := "SELECT count(*) FILTER (WHERE tenant_id=$1 AND idem_key LIKE 'old-%'), " +
				"count(*) FILTER (WHERE tenant_id=$1 AND idem_key='recent'), " +
				"count(*) FILTER (WHERE tenant_id=$2 AND idem_key='recent') FROM " + table.name
			if err := pool.QueryRow(ctx, query, tenant, otherTenant).Scan(&old, &recent, &other); err != nil {
				t.Fatal(err)
			}
			if old != 0 || recent != 1 || other != 1 {
				t.Fatalf("%s retention: old=%d recent=%d second tenant=%d", table.name, old, recent, other)
			}
		}
	}
	// The maintenance may delete only keys, never the underlying transactions.
	var sales, returns int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM sales WHERE id IN ($1,$2)", sale, otherSale).Scan(&sales); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM sale_returns WHERE id IN ($1,$2)", returned, otherReturn).Scan(&returns); err != nil {
		t.Fatal(err)
	}
	if sales != 2 || returns != 2 {
		t.Fatal("retention destroyed underlying commercial records")
	}
}
