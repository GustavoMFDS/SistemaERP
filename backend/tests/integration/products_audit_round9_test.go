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
	invapp "github.com/example/sistemaemgo/internal/modules/inventory/application"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProductMutationRollsBackWhenAuditActorIsOutsideTenant(t *testing.T) {
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

	var tenantA, actorA string
	if err := pool.QueryRow(ctx, `
		SELECT ut.tenant_id::text, ut.user_id::text
		FROM user_tenants ut
		JOIN users u ON u.id=ut.user_id
		WHERE ut.active=true AND u.active=true
		ORDER BY ut.created_at
		LIMIT 1
	`).Scan(&tenantA, &actorA); err != nil {
		t.Fatalf("seeded tenant/actor required: %v", err)
	}

	tenantB := round7CreateTenant(t, ctx, pool, "Round9 Product Audit Tenant B")
	email := fmt.Sprintf("round9-product-%d@example.test", time.Now().UnixNano())
	var actorB string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users(email, name, password_hash, active)
		VALUES ($1,'Round9 Foreign Actor','not-used',true)
		RETURNING id::text
	`, email).Scan(&actorB); err != nil {
		t.Fatalf("create foreign actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_tenants(user_id, tenant_id)
		VALUES ($1,$2)
	`, actorB, tenantB); err != nil {
		t.Fatalf("create foreign membership: %v", err)
	}

	var productID string
	t.Cleanup(func() {
		bg := context.Background()
		if productID != "" {
			_, _ = pool.Exec(bg, `DELETE FROM audit_logs WHERE resource_id=$1::uuid`, productID)
			_, _ = pool.Exec(bg, `DELETE FROM products WHERE id=$1`, productID)
		}
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id=$1`, actorB)
		_, _ = pool.Exec(bg, `DELETE FROM companies WHERE id=$1`, tenantB)
	})

	repo := invinfra.NewProductsRepo(pool)
	auditSvc := audit.New(pool, slog.Default())
	svc := invapp.NewProductsService(
		db.NewPgxUnitOfWork(pool),
		repo,
		auditSvc,
		validator.New(),
		slog.Default(),
	)

	suffix := time.Now().UnixNano()
	req := invapp.ProductCreateRequest{
		SKU:       fmt.Sprintf("R9-PROD-AUDIT-%d", suffix),
		Name:      "Round9 Product Audit",
		Unit:      "UN",
		CostPrice: platform.NewMoneyCents(100),
		PriceCash: platform.NewMoneyCents(250),
		MinStock:  platform.NewQuantityMilli(0),
		Active:    true,
	}

	if _, err := svc.Create(ctx, tenantA, actorB, req); err == nil {
		t.Fatal("foreign audit actor must make product create fail")
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM products
		WHERE tenant_id=$1 AND sku=$2
	`, tenantA, req.SKU).Scan(&count); err != nil {
		t.Fatalf("count rolled-back product: %v", err)
	}
	if count != 0 {
		t.Fatalf("product create committed without valid audit actor: count=%d", count)
	}

	productID, err = svc.Create(ctx, tenantA, actorA, req)
	if err != nil {
		t.Fatalf("valid product create: %v", err)
	}

	update := req
	update.Name = "Round9 Product Audit Updated"
	update.PriceCash = platform.NewMoneyCents(300)

	if err := svc.Update(ctx, tenantA, actorB, productID, update); err == nil {
		t.Fatal("foreign audit actor must make product update fail")
	}

	var name, price string
	if err := pool.QueryRow(ctx, `
		SELECT name, price_cash::text
		FROM products
		WHERE tenant_id=$1 AND id=$2
	`, tenantA, productID).Scan(&name, &price); err != nil {
		t.Fatalf("read rolled-back update: %v", err)
	}
	if name != req.Name || price != "2.50" {
		t.Fatalf("product update committed without audit: name=%q price=%s", name, price)
	}

	if err := svc.Update(ctx, tenantA, actorA, productID, update); err != nil {
		t.Fatalf("valid product update: %v", err)
	}

	var createAudits, updateAudits int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM audit_logs
			  WHERE tenant_id=$1 AND resource_id=$2 AND action='product.create'),
			(SELECT count(*) FROM audit_logs
			  WHERE tenant_id=$1 AND resource_id=$2 AND action='product.update')
	`, tenantA, productID).Scan(&createAudits, &updateAudits); err != nil {
		t.Fatalf("read product audit evidence: %v", err)
	}
	if createAudits != 1 || updateAudits != 1 {
		t.Fatalf("product audits create=%d update=%d, want 1/1", createAudits, updateAudits)
	}
}
