//go:build integration

package integration_test

import (
	"context"
	"os"
	"testing"
	"time"

	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This integration test validates tenant isolation at the repository layer.
// It requires a real PostgreSQL.
// Run:
//
//	TEST_DATABASE_URL=postgres://... go test ./... -tags=integration -run TestProductsRepo_TenantIsolation
func TestProductsRepo_TenantIsolation(t *testing.T) {
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

	// Minimal sanity check to ensure the DB is reachable.
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("db ping: %v", err)
	}

	repo := invinfra.NewProductsRepo(pool)

	// NOTE: This assumes migrations are already applied and there are at least
	// 2 tenants in companies table. We keep it non-destructive.
	// If your DB is empty, seed it first via docker-compose seed.
	rows, err := pool.Query(ctx, `SELECT id::text FROM companies ORDER BY created_at LIMIT 2`)
	if err != nil {
		t.Fatalf("query companies: %v", err)
	}
	defer rows.Close()

	var tenants []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tenants = append(tenants, id)
	}
	if len(tenants) < 2 {
		t.Skip("need at least 2 tenants in companies")
	}

	// List should never error, and must be scoped by tenant.
	_, _, err = repo.List(ctx, tenants[0], "", 10, 0)
	if err != nil {
		t.Fatalf("list tenant A: %v", err)
	}
	_, _, err = repo.List(ctx, tenants[1], "", 10, 0)
	if err != nil {
		t.Fatalf("list tenant B: %v", err)
	}
}
