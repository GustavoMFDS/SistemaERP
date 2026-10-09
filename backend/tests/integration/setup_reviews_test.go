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
	"github.com/example/sistemaemgo/internal/modules/setup"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSetupReviewsStayInTheirCompanyAndHaveTransactionalAudit(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	var actor string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM users WHERE active=true LIMIT 1`).Scan(&actor); err != nil {
		t.Fatalf("integration seed user required: %v", err)
	}
	tenants := make([]string, 0, 2)
	now := time.Now().UnixNano()
	for i := 0; i < 2; i++ {
		cnpj := fmt.Sprintf("%014d", (now + int64(i)) % 100000000000000)
		var tenant string
		if err := pool.QueryRow(ctx, `
			INSERT INTO companies(legal_name, cnpj) VALUES('Setup Review Fixture', $1)
			RETURNING id::text
		`, cnpj).Scan(&tenant); err != nil {
			t.Fatal(err)
		}
		tenants = append(tenants, tenant)
		if _, err := pool.Exec(ctx, `INSERT INTO user_tenants(user_id,tenant_id) VALUES($1,$2)`, actor, tenant); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		for _, tenant := range tenants {
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM audit_logs WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM setup_step_reviews WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM user_tenant_roles WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM user_tenants WHERE tenant_id=$1`, tenant)
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM companies WHERE id=$1`, tenant)
		}
	}()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := setup.New(pool, audit.New(pool, logger))
	a, b := tenants[0], tenants[1]
	if err := svc.Set(ctx, a, actor, "stock", true, "setup-test", "127.0.0.1", "integration"); err != nil {
		t.Fatal(err)
	}

	anotherInstance := setup.New(pool, audit.New(pool, logger))
	one, err := anotherInstance.List(ctx, a)
	if err != nil || len(one) != 1 || one[0].Step != "stock" {
		t.Fatalf("review must persist in company A: %+v, %v", one, err)
	}
	other, err := anotherInstance.List(ctx, b)
	if err != nil || len(other) != 0 {
		t.Fatalf("company B must not see A review: %+v, %v", other, err)
	}
	if err := svc.Set(ctx, b, actor, "team", true, "setup-test", "127.0.0.1", "integration"); err != nil {
		t.Fatal(err)
	}
	one, err = svc.List(ctx, a)
	if err != nil || len(one) != 1 || one[0].Step != "stock" {
		t.Fatalf("B affected A: %+v, %v", one, err)
	}

	if err := svc.Set(ctx, a, actor, "fiscal", true, "setup-test", "127.0.0.1", "integration"); !errors.Is(err, setup.ErrInvalidStep) {
		t.Fatalf("must reject manual fiscal approval: %v", err)
	}
	if err := svc.Set(ctx, a, actor, "stock", false, "setup-test", "127.0.0.1", "integration"); err != nil {
		t.Fatal(err)
	}
	one, err = svc.List(ctx, a)
	if err != nil || len(one) != 0 {
		t.Fatalf("revocation not persisted: %+v, %v", one, err)
	}

	var auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE tenant_id=$1 AND action='setup.review.set'
	`, a).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("expected transactional audit of review and reopening, got %d", auditCount)
	}

	if err := svc.Set(ctx, a, "00000000-0000-4000-8000-000000000001", "team", true, "setup-test", "127.0.0.1", "integration"); err == nil {
		t.Fatal("actor without tenant membership must fail FK, not create an unowned review")
	}
	one, err = svc.List(ctx, a)
	if err != nil || len(one) != 0 {
		t.Fatalf("failed write was not rolled back: %+v, %v", one, err)
	}
}
