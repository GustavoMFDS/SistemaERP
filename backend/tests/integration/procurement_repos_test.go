//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	proc "github.com/example/sistemaemgo/internal/modules/procurement/domain"
	procinfra "github.com/example/sistemaemgo/internal/modules/procurement/infrastructure"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProcurementRepo_TenantIsolation(t *testing.T) {
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

	var tenantA, productA, actorA string
	if err := pool.QueryRow(ctx, `
		SELECT p.tenant_id::text, p.id::text, utr.user_id::text
		FROM products p
		JOIN user_tenant_roles utr ON utr.tenant_id=p.tenant_id
		JOIN roles r ON r.id=utr.role_id AND r.name='admin'
		ORDER BY p.created_at
		LIMIT 1
	`).Scan(&tenantA, &productA, &actorA); err != nil {
		t.Fatalf("seeded tenant/product/admin required: %v", err)
	}

	cnpj := fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000)
	var tenantB string
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies(legal_name, trade_name, cnpj)
		VALUES ('Procurement Tenant B', 'Procurement Tenant B', $1)
		RETURNING id::text
	`, cnpj).Scan(&tenantB); err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	repo := procinfra.NewRepo(pool)
	uow := db.NewPgxUnitOfWork(pool)
	// A fresh document avoids collisions with fixtures left by interrupted runs.
	document := fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000)

	// Install cleanup before any write; failures halfway through supplier or
	// purchase creation must not leave another conflicting tenant fixture.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM purchase_items WHERE purchase_id IN (SELECT id FROM purchases WHERE tenant_id=$1 AND supplier_id IN (SELECT id FROM suppliers WHERE tenant_id=$1 AND document=$2))`, tenantA, document)
		_, _ = pool.Exec(context.Background(), `DELETE FROM purchases WHERE tenant_id=$1 AND supplier_id IN (SELECT id FROM suppliers WHERE tenant_id=$1 AND document=$2)`, tenantA, document)
		_, _ = pool.Exec(context.Background(), `DELETE FROM suppliers WHERE tenant_id=$1 AND document=$3 OR tenant_id=$2 AND document=$3`, tenantA, tenantB, document)
		_, _ = pool.Exec(context.Background(), `DELETE FROM companies WHERE id=$1`, tenantB)
	})
	createSupplier := func(tenantID, name string) string {
		t.Helper()
		tx, err := uow.Begin(ctx)
		if err != nil {
			t.Fatalf("begin supplier tx: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		id, err := repo.CreateSupplier(ctx, tx, tenantID, proc.Supplier{
			Name: name, Document: &document, Active: true,
		})
		if err != nil {
			t.Fatalf("create supplier %s: %v", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit supplier %s: %v", name, err)
		}
		return id
	}

	supplierA := createSupplier(tenantA, "Fornecedor Tenant A")
	supplierB := createSupplier(tenantB, "Fornecedor Tenant B")

	// The database must reject cross-tenant references even if a caller bypasses
	// the application service and writes directly to the procurement tables.
	if _, err := pool.Exec(ctx, `
		INSERT INTO purchases(tenant_id, supplier_id, status, total, created_by_user_id)
		VALUES ($1,$2,'ordered',1,$3)
	`, tenantA, supplierB, actorA); err == nil {
		t.Fatal("database unexpectedly accepted a cross-tenant supplier on purchase")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO accounts_payable(tenant_id, description, amount, due_date, supplier_id)
		VALUES ($1,'Cross-tenant supplier payable',1,current_date,$2)
	`, tenantA, supplierB); err == nil {
		t.Fatal("database unexpectedly accepted a cross-tenant supplier on accounts payable")
	}



	listA, totalA, err := repo.ListSuppliers(ctx, tenantA, "Fornecedor Tenant", 50, 0)
	if err != nil {
		t.Fatalf("list tenant A suppliers: %v", err)
	}
	listB, totalB, err := repo.ListSuppliers(ctx, tenantB, "Fornecedor Tenant", 50, 0)
	if err != nil {
		t.Fatalf("list tenant B suppliers: %v", err)
	}
	if totalA != 1 || len(listA) != 1 || listA[0].ID != supplierA {
		t.Fatalf("tenant A supplier isolation failed: total=%d items=%v", totalA, listA)
	}
	if totalB != 1 || len(listB) != 1 || listB[0].ID != supplierB {
		t.Fatalf("tenant B supplier isolation failed: total=%d items=%v", totalB, listB)
	}

	tx, err := uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin duplicate supplier tx: %v", err)
	}
	_, duplicateErr := repo.CreateSupplier(ctx, tx, tenantA, proc.Supplier{
		Name: "Duplicate Tenant A", Document: &document, Active: true,
	})
	_ = tx.Rollback(ctx)
	if duplicateErr == nil {
		t.Fatal("same supplier document inside one tenant must be rejected")
	}

	qty := platform.NewQuantityMilli(2000)
	unitCost := platform.NewMoneyCents(750)
	lineTotal := unitCost.MulQty(qty)
	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin purchase tx: %v", err)
	}
	purchaseID, err := repo.CreatePurchase(ctx, tx, tenantA, proc.Purchase{
		SupplierID: supplierA,
		Status:     proc.PurchaseOrdered,
		Total:      lineTotal,
		CreatedBy:  actorA,
	}, []proc.PurchaseItem{{
		ProductID:  productA,
		QtyOrdered: qty,
		UnitCost:   unitCost,
		LineTotal:  lineTotal,
	}})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("create purchase tenant A: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit purchase tenant A: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO accounts_payable(tenant_id, description, amount, due_date, purchase_id)
		VALUES ($1,'Cross-tenant purchase payable',1,current_date,$2)
	`, tenantB, purchaseID); err == nil {
		t.Fatal("database unexpectedly accepted a cross-tenant purchase on accounts payable")
	}

	if _, _, _, err := repo.GetPurchase(ctx, tenantB, purchaseID); err == nil {
		t.Fatal("tenant B unexpectedly fetched tenant A purchase")
	}
}
