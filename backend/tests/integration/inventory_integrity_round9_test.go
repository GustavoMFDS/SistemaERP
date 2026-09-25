//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
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

func TestInventoryAdjustmentIsTenantScopedAndAuditedAtomically(t *testing.T) {
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

	var tenantA, userID string
	if err := pool.QueryRow(ctx, `
		SELECT ut.tenant_id::text, ut.user_id::text
		FROM user_tenants ut
		JOIN users u ON u.id=ut.user_id
		WHERE ut.active=true AND u.active=true
		ORDER BY ut.created_at
		LIMIT 1
	`).Scan(&tenantA, &userID); err != nil {
		t.Fatalf("seeded tenant/user required: %v", err)
	}

	suffix := time.Now().UnixNano()
	var productID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO products(
			tenant_id, sku, name, unit, cost_price, price_cash, min_stock, active
		)
		VALUES ($1,$2,'Round9 Inventory Product','UN',1,2,0,true)
		RETURNING id::text
	`, tenantA, fmt.Sprintf("R9-INV-%d", suffix)).Scan(&productID); err != nil {
		t.Fatalf("create product: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO inventory_balances(tenant_id, product_id, qty_on_hand)
		VALUES ($1,$2,0)
	`, tenantA, productID); err != nil {
		t.Fatalf("create balance: %v", err)
	}

	tenantB := round7CreateTenant(t, ctx, pool, "Round9 Inventory Tenant B")
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM audit_logs WHERE resource_id=$1::uuid`, productID)
		_, _ = pool.Exec(bg, `DELETE FROM inventory_movements WHERE product_id=$1`, productID)
		_, _ = pool.Exec(bg, `DELETE FROM inventory_balances WHERE product_id=$1`, productID)
		_, _ = pool.Exec(bg, `DELETE FROM products WHERE id=$1`, productID)
		_, _ = pool.Exec(bg, `DELETE FROM companies WHERE id=$1`, tenantB)
	})

	productsRepo := invinfra.NewProductsRepo(pool)
	inventoryRepo := invinfra.NewInventoryRepo(pool)
	auditSvc := audit.New(pool, slog.Default())
	svc := invapp.NewInventoryService(
		config.Config{},
		db.NewPgxUnitOfWork(pool),
		inventoryRepo,
		productsRepo,
		auditSvc,
		validator.New(),
		slog.Default(),
	)

	req := invapp.InventoryAdjustRequest{
		ProductID: productID,
		Delta:     platform.NewQuantityMilli(2_000),
		Reason:    "round9 tenant integrity",
		Type:      "adjustment",
	}

	if err := svc.Adjust(ctx, tenantB, userID, req); !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("cross-tenant adjust must return ErrNotFound, got %v", err)
	}

	var crossBalances, crossMovements, crossAudits int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM inventory_balances WHERE tenant_id=$1 AND product_id=$2),
			(SELECT count(*) FROM inventory_movements WHERE tenant_id=$1 AND product_id=$2),
			(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND resource_id=$2 AND action='inventory.adjust')
	`, tenantB, productID).Scan(&crossBalances, &crossMovements, &crossAudits); err != nil {
		t.Fatalf("read cross-tenant side effects: %v", err)
	}
	if crossBalances != 0 || crossMovements != 0 || crossAudits != 0 {
		t.Fatalf(
			"cross-tenant adjust leaked side effects balances=%d movements=%d audits=%d",
			crossBalances,
			crossMovements,
			crossAudits,
		)
	}

	if err := svc.Adjust(ctx, tenantA, userID, req); err != nil {
		t.Fatalf("valid tenant adjust: %v", err)
	}

	var qty string
	if err := pool.QueryRow(ctx, `
		SELECT qty_on_hand::text
		FROM inventory_balances
		WHERE tenant_id=$1 AND product_id=$2
	`, tenantA, productID).Scan(&qty); err != nil {
		t.Fatalf("read adjusted balance: %v", err)
	}
	if qty != "2.000" {
		t.Fatalf("adjusted balance=%s, want 2.000", qty)
	}

	var movements, audits int
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM inventory_movements WHERE tenant_id=$1 AND product_id=$2),
			(SELECT count(*) FROM audit_logs WHERE tenant_id=$1 AND resource_id=$2 AND action='inventory.adjust')
	`, tenantA, productID).Scan(&movements, &audits); err != nil {
		t.Fatalf("read committed adjustment evidence: %v", err)
	}
	if movements != 1 || audits != 1 {
		t.Fatalf("adjustment evidence movements=%d audits=%d, want 1/1", movements, audits)
	}
}
