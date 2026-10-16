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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProductVariationAtomicStockAndTenantBoundary(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var tenant, actor, otherTenant string
	if err := pool.QueryRow(ctx, `
        SELECT ut.tenant_id::text, ut.user_id::text
        FROM user_tenant_roles ut JOIN roles r ON r.id=ut.role_id
        WHERE r.name='admin' ORDER BY ut.created_at LIMIT 1
    `).Scan(&tenant, &actor); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
        SELECT id::text FROM companies WHERE id<>$1 ORDER BY created_at LIMIT 1
    `, tenant).Scan(&otherTenant); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := invapp.NewProductsService(db.NewPgxUnitOfWork(pool),
		invinfra.NewProductsRepo(pool), audit.New(pool, logger), validator.New(), logger)
	suffix := uuid.NewString()
	product := func(sku, name string) invapp.ProductCreateRequest {
		return invapp.ProductCreateRequest{
			SKU: sku, Name: name, Unit: "un",
			PriceCash: platform.NewMoneyCents(1290), Active: true,
		}
	}
	parentSKU := "FAMILY-" + suffix
	parent, err := svc.Create(ctx, tenant, actor, product(parentSKU, "Caderno"))
	if err != nil {
		t.Fatal(err)
	}
	var child string
	t.Cleanup(func() {
		c, stop := context.WithTimeout(context.Background(), 8*time.Second)
		defer stop()
		for _, id := range []string{child, parent} {
			if id == "" {
				continue
			}
			_, _ = pool.Exec(c, `DELETE FROM audit_logs WHERE tenant_id=$1 AND resource_id=$2`, tenant, id)
			_, _ = pool.Exec(c, `DELETE FROM product_variations WHERE tenant_id=$1 AND (parent_product_id=$2 OR variant_product_id=$2)`, tenant, id)
			_, _ = pool.Exec(c, `DELETE FROM inventory_balances WHERE tenant_id=$1 AND product_id=$2`, tenant, id)
			_, _ = pool.Exec(c, `DELETE FROM products WHERE tenant_id=$1 AND id=$2`, tenant, id)
		}
	})
	// Give the parent existing stock; creating the color may not redistribute it.
	if _, err := pool.Exec(ctx, `
        UPDATE inventory_balances SET qty_on_hand=15
        WHERE tenant_id=$1 AND product_id=$2
    `, tenant, parent); err != nil {
		t.Fatal(err)
	}

	childSKU := "FAMILY-AZUL-" + suffix
	child, err = svc.CreateVariation(ctx, tenant, actor, parent, " Azul ",
		product(childSKU, "Caderno — Azul"))
	if err != nil {
		t.Fatalf("create atomic variant: %v", err)
	}
	var linkedLabel string
	if err := pool.QueryRow(ctx, `
        SELECT option_label FROM product_variations
        WHERE tenant_id=$1 AND parent_product_id=$2 AND variant_product_id=$3
    `, tenant, parent, child).Scan(&linkedLabel); err != nil {
		t.Fatal(err)
	}
	if linkedLabel != "Azul" {
		t.Fatalf("option label: %q", linkedLabel)
	}
	var parentQty, childQty string
	if err := pool.QueryRow(ctx, `
        SELECT qty_on_hand::text FROM inventory_balances
        WHERE tenant_id=$1 AND product_id=$2
    `, tenant, parent).Scan(&parentQty); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
        SELECT qty_on_hand::text FROM inventory_balances
        WHERE tenant_id=$1 AND product_id=$2
    `, tenant, child).Scan(&childQty); err != nil {
		t.Fatal(err)
	}
	if parentQty != "15.000" && parentQty != "15" {
		t.Fatalf("parent stock changed: %s", parentQty)
	}
	if childQty != "0.000" && childQty != "0" {
		t.Fatalf("variant should start zero: %s", childQty)
	}

	// Duplicate color and nested variant must fail and roll back new SKUs.
	duplicateSKU := "FAMILY-DUP-" + suffix
	_, err = svc.CreateVariation(ctx, tenant, actor, parent, "Azul",
		product(duplicateSKU, "Caderno — Duplicate"))
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("duplicate option should conflict, got %v", err)
	}
	var created int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM products WHERE tenant_id=$1 AND sku=$2`,
		tenant, duplicateSKU).Scan(&created); err != nil {
		t.Fatal(err)
	}
	if created != 0 {
		t.Fatal("failed variation left orphaned SKU")
	}

	_, err = svc.CreateVariation(ctx, tenant, actor, child, "Listrado",
		product("FAMILY-NEST-"+suffix, "Nested"))
	if !errors.Is(err, common.ErrConflict) {
		t.Fatalf("nested group should conflict, got %v", err)
	}
	_, err = svc.CreateVariation(ctx, otherTenant, actor, parent, "Verde",
		product("FAMILY-FOREIGN-"+suffix, "Foreign"))
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("cross-tenant parent should not be found, got %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM product_variations
        WHERE tenant_id=$1 AND parent_product_id=$2`, tenant, parent).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal(fmt.Sprintf("unexpected family size %d", count))
	}
}
