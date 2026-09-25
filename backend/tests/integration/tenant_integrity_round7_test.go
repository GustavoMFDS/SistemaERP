//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	authinfra "github.com/example/sistemaemgo/internal/modules/auth/infrastructure"
	"github.com/example/sistemaemgo/internal/modules/common"
	invinfra "github.com/example/sistemaemgo/internal/modules/inventory/infrastructure"
	privacyinfra "github.com/example/sistemaemgo/internal/modules/privacy/infrastructure"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRound7TenantRelationalIntegrity(t *testing.T) {
	ctx, pool := round7IntegrationDB(t)

	var tenantA, existingProductID string
	if err := pool.QueryRow(ctx, `
		SELECT tenant_id::text, id::text
		FROM products
		ORDER BY created_at
		LIMIT 1
	`).Scan(&tenantA, &existingProductID); err != nil {
		t.Fatalf("seeded tenant/product required: %v", err)
	}

	tenantB := round7CreateTenant(t, ctx, pool, "Round7 Tenant B")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM companies WHERE id=$1`, tenantB)
	})

	repo := invinfra.NewProductsRepo(pool)
	existing, err := repo.Get(ctx, tenantA, existingProductID)
	if err != nil {
		t.Fatalf("get tenant A product: %v", err)
	}
	updateTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin update tx: %v", err)
	}
	err = repo.Update(ctx, updateTx, tenantB, existingProductID, existing)
	_ = updateTx.Rollback(ctx)
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("cross-tenant/missing product update must return ErrNotFound, got %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin integrity tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	suffix := time.Now().UnixNano()
	barcode := fmt.Sprintf("9%012d", suffix%1_000_000_000_000)
	if _, err := tx.Exec(ctx, `
		INSERT INTO products(tenant_id, sku, barcode, name, unit, cost_price, price_cash, min_stock, active)
		VALUES ($1,$2,$3,'Round7 Produto A','UN',1,2,0,true)
	`, tenantA, fmt.Sprintf("R7-A-%d", suffix), barcode); err != nil {
		t.Fatalf("insert tenant A barcode fixture: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO products(tenant_id, sku, barcode, name, unit, cost_price, price_cash, min_stock, active)
		VALUES ($1,$2,$3,'Round7 Produto B','UN',1,2,0,true)
	`, tenantB, fmt.Sprintf("R7-B-%d", suffix), barcode); err != nil {
		t.Fatalf("same barcode must be allowed in independent tenants: %v", err)
	}

	var categoryA string
	if err := tx.QueryRow(ctx, `
		INSERT INTO categories(tenant_id, name)
		VALUES ($1,$2)
		RETURNING id::text
	`, tenantA, fmt.Sprintf("Round7 Categoria %d", suffix)).Scan(&categoryA); err != nil {
		t.Fatalf("create tenant A category: %v", err)
	}

	if _, err := tx.Exec(ctx, `SAVEPOINT cross_category`); err != nil {
		t.Fatalf("savepoint category: %v", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO products(tenant_id, category_id, sku, name, unit, cost_price, price_cash, min_stock, active)
		VALUES ($1,$2,$3,'Cross Tenant Category','UN',1,2,0,true)
	`, tenantB, categoryA, fmt.Sprintf("R7-XCAT-%d", suffix))
	round7RequireConstraint(t, err, "products_tenant_category_fk")
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT cross_category`); err != nil {
		t.Fatalf("rollback category savepoint: %v", err)
	}

	var customerA string
	if err := tx.QueryRow(ctx, `
		INSERT INTO customers(tenant_id, name)
		VALUES ($1,'Round7 Cliente A')
		RETURNING id::text
	`, tenantA).Scan(&customerA); err != nil {
		t.Fatalf("create tenant A customer: %v", err)
	}
	var userID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM users ORDER BY created_at LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("seeded user required: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_tenants(user_id, tenant_id)
		VALUES ($1,$2)
		ON CONFLICT (user_id, tenant_id) DO UPDATE SET active=true
	`, userID, tenantB); err != nil {
		t.Fatalf("create tenant B actor membership: %v", err)
	}

	var registerB string
	if err := tx.QueryRow(ctx, `
		INSERT INTO cash_registers(tenant_id, name, active)
		VALUES ($1,$2,true)
		RETURNING id::text
	`, tenantB, fmt.Sprintf("Round7 Caixa %d", suffix)).Scan(&registerB); err != nil {
		t.Fatalf("create tenant B register: %v", err)
	}
	var sessionB string
	if err := tx.QueryRow(ctx, `
		INSERT INTO cash_sessions(tenant_id, cash_register_id, opened_by_user_id, opening_amount, status)
		VALUES ($1,$2,$3,0,'open')
		RETURNING id::text
	`, tenantB, registerB, userID).Scan(&sessionB); err != nil {
		t.Fatalf("create tenant B session: %v", err)
	}

	if _, err := tx.Exec(ctx, `SAVEPOINT cross_customer`); err != nil {
		t.Fatalf("savepoint customer: %v", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO sales(
			tenant_id, cash_session_id, customer_id, status, subtotal,
			discount_value, total, profit_estimated, created_by_user_id
		)
		VALUES ($1,$2,$3,'finalized',10,0,10,1,$4)
	`, tenantB, sessionB, customerA, userID)
	round7RequireConstraint(t, err, "sales_tenant_customer_fk")
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT cross_customer`); err != nil {
		t.Fatalf("rollback customer savepoint: %v", err)
	}

	if _, err := tx.Exec(ctx, `SAVEPOINT orphan_tenant`); err != nil {
		t.Fatalf("savepoint orphan: %v", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO products(tenant_id, sku, name, unit, cost_price, price_cash, min_stock, active)
		VALUES ($1,$2,'Orphan Tenant Product','UN',1,2,0,true)
	`, uuid.NewString(), fmt.Sprintf("R7-ORPHAN-%d", suffix))
	round7RequireConstraint(t, err, "products_tenant_fk")
}

func TestRound7TenantScopedUserPrivacyActions(t *testing.T) {
	ctx, pool := round7IntegrationDB(t)

	tenantA := round7CreateTenant(t, ctx, pool, "Round7 Privacy A")
	tenantB := round7CreateTenant(t, ctx, pool, "Round7 Privacy B")
	email := fmt.Sprintf("round7-%d@example.test", time.Now().UnixNano())

	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users(email, name, password_hash, active)
		VALUES ($1,'Round7 Shared User','not-used',true)
		RETURNING id::text
	`, email).Scan(&userID); err != nil {
		t.Fatalf("create shared user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM companies WHERE id IN ($1,$2)`, tenantA, tenantB)
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})

	if _, err := pool.Exec(ctx, `
		INSERT INTO user_tenants(user_id, tenant_id)
		VALUES ($1,$2),($1,$3)
	`, userID, tenantA, tenantB); err != nil {
		t.Fatalf("create memberships: %v", err)
	}

	privacyRepo := privacyinfra.NewRepo(pool)
	authRepo := authinfra.NewUsersRepo(pool, false)

	if err := privacyRepo.BlockSubject(ctx, tenantA, "user", userID); err != nil {
		t.Fatalf("block tenant A membership: %v", err)
	}

	var globalActive, tenantAActive, tenantBActive bool
	if err := pool.QueryRow(ctx, `
		SELECT
			u.active,
			(SELECT active FROM user_tenants WHERE user_id=u.id AND tenant_id=$2),
			(SELECT active FROM user_tenants WHERE user_id=u.id AND tenant_id=$3)
		FROM users u
		WHERE u.id=$1
	`, userID, tenantA, tenantB).Scan(&globalActive, &tenantAActive, &tenantBActive); err != nil {
		t.Fatalf("read membership state: %v", err)
	}
	if !globalActive || tenantAActive || !tenantBActive {
		t.Fatalf("unexpected scoped block state global=%v tenantA=%v tenantB=%v", globalActive, tenantAActive, tenantBActive)
	}

	hasA, err := authRepo.UserHasTenant(ctx, userID, tenantA)
	if err != nil {
		t.Fatalf("tenant A access check: %v", err)
	}
	hasB, err := authRepo.UserHasTenant(ctx, userID, tenantB)
	if err != nil {
		t.Fatalf("tenant B access check: %v", err)
	}
	if hasA || !hasB {
		t.Fatalf("block must revoke only tenant A: hasA=%v hasB=%v", hasA, hasB)
	}

	tenants, err := authRepo.ListUserTenants(ctx, userID)
	if err != nil {
		t.Fatalf("list active memberships: %v", err)
	}
	if len(tenants) != 1 || tenants[0].ID != tenantB {
		t.Fatalf("expected only tenant B after scoped block, got %#v", tenants)
	}

	if err := privacyRepo.AnonymizeSubject(ctx, tenantB, "user", userID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("shared global identity anonymization must conflict, got %v", err)
	}

	var persistedEmail string
	if err := pool.QueryRow(ctx, `SELECT email::text FROM users WHERE id=$1`, userID).Scan(&persistedEmail); err != nil {
		t.Fatalf("read shared user email: %v", err)
	}
	if persistedEmail != email {
		t.Fatalf("conflicted anonymization changed shared identity: got %q want %q", persistedEmail, email)
	}
}

func round7IntegrationDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("db ping: %v", err)
	}
	return ctx, pool
}

func round7CreateTenant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) string {
	t.Helper()
	cnpj := fmt.Sprintf("%014d", time.Now().UnixNano()%100_000_000_000_000)
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies(legal_name, trade_name, cnpj)
		VALUES ($1,$1,$2)
		RETURNING id::text
	`, name, cnpj).Scan(&id); err != nil {
		t.Fatalf("create tenant %q: %v", name, err)
	}
	return id
}

func round7RequireConstraint(t *testing.T, err error, constraint string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected constraint %s to reject cross-tenant write", constraint)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected PostgreSQL constraint error for %s, got %T: %v", constraint, err, err)
	}
	if pgErr.Code != "23503" || pgErr.ConstraintName != constraint {
		t.Fatalf("unexpected constraint error: code=%s constraint=%s want=%s", pgErr.Code, pgErr.ConstraintName, constraint)
	}
}
