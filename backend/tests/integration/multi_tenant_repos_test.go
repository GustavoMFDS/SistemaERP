//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This integration test validates tenant isolation against a real PostgreSQL.
// Migrations and the demo seed must be applied before running it.
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

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("db ping: %v", err)
	}

	var tenantA string
	if err := pool.QueryRow(ctx, `
		SELECT tenant_id::text
		FROM products
		ORDER BY created_at
		LIMIT 1
	`).Scan(&tenantA); err != nil {
		t.Fatalf("seeded tenant with product required: %v", err)
	}

	cnpj := fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000)
	var tenantB string
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies(legal_name, trade_name, cnpj)
		VALUES ('Integration Tenant Isolation', 'Integration Tenant Isolation', $1)
		RETURNING id::text
	`, cnpj).Scan(&tenantB); err != nil {
		t.Fatalf("create tenant B: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM companies WHERE id=$1`, tenantB)
	})

	repo := invinfra.NewProductsRepo(pool)

	itemsA, totalA, err := repo.List(ctx, tenantA, "", 50, 0)
	if err != nil {
		t.Fatalf("list tenant A: %v", err)
	}
	if totalA == 0 || len(itemsA) == 0 {
		t.Fatal("tenant A must contain the seeded product")
	}

	itemsB, totalB, err := repo.List(ctx, tenantB, "", 50, 0)
	if err != nil {
		t.Fatalf("list tenant B: %v", err)
	}
	if totalB != 0 || len(itemsB) != 0 {
		t.Fatalf("tenant B leaked tenant A products: total=%d items=%d", totalB, len(itemsB))
	}

	if _, err := repo.Get(ctx, tenantB, itemsA[0].ID); err == nil {
		t.Fatal("tenant B unexpectedly fetched tenant A product by id")
	}
}
