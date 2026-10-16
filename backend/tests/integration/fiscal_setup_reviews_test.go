//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/modules/audit"
	"github.com/example/sistemaemgo/internal/modules/setup"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Requires an isolated disposable database with migrations up through 0045.
// These acknowledgements NEVER allocate invoice numbers or contact SEFAZ.
func TestFiscalSetupReviewsTenantBoundaryAndAudit(t *testing.T) {
	if os.Getenv("TEST_FISCAL_DISPOSABLE_DB") != "1" {
		t.Skip("requires an explicitly disposable fiscal test database")
	}
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

	var seededTenant, actor string
	if err := pool.QueryRow(ctx, `
		SELECT utr.tenant_id::text,utr.user_id::text FROM user_tenant_roles utr
		JOIN roles r ON r.id=utr.role_id AND r.name='admin'
		ORDER BY utr.user_id LIMIT 1
	`).Scan(&seededTenant, &actor); err != nil {
		t.Fatal(err)
	}
	// The test must not mutate the real seeded company's setup checklist.
	companyA, companyB := "", ""
	defer func() {
		for _, id := range []string{companyA, companyB} {
			if id != "" {
				_, _ = pool.Exec(context.Background(), `DELETE FROM companies WHERE id=$1`, id)
			}
		}
	}()
	for i, dest := range []*string{&companyA, &companyB} {
		cnpj := fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000+int64(i))
		if err := pool.QueryRow(ctx, `
			INSERT INTO companies(legal_name,cnpj)
			VALUES($1,$2) RETURNING id::text
		`, "Fiscal Review Test "+uuid.NewString(), cnpj).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_tenants(user_id,tenant_id) VALUES($1,$2)
	`, actor, companyA); err != nil {
		t.Fatal(err)
	}

	svc := setup.New(pool, audit.New(pool, nil))
	step := "fiscal_docs_collected"
	if err := svc.Set(ctx, companyA, actor, step, true, "", "", "integration"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Set(ctx, companyB, actor, step, true, "", "", "integration"); err == nil {
		t.Fatal("review was attributed to an actor not associated with company B")
	}
	gotA, err := svc.List(ctx, companyA)
	if err != nil || len(gotA) != 1 || gotA[0].Step != step {
		t.Fatalf("company A review missing: %+v %v", gotA, err)
	}
	gotB, err := svc.List(ctx, companyB)
	if err != nil || len(gotB) != 0 {
		t.Fatalf("review leaked to company B: %+v %v", gotB, err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO user_tenants(user_id,tenant_id) VALUES($1,$2)
	`, actor, companyB); err != nil {
		t.Fatal(err)
	}
	if err := svc.Set(ctx, companyB, actor, "fiscal_accountant_consulted", true, "", "", "integration"); err != nil {
		t.Fatal(err)
	}
	gotA, err = svc.List(ctx, companyA)
	if err != nil || len(gotA) != 1 || gotA[0].Step != step {
		t.Fatalf("company B affected company A: %+v %v", gotA, err)
	}
	gotB, err = svc.List(ctx, companyB)
	if err != nil || len(gotB) != 1 || gotB[0].Step != "fiscal_accountant_consulted" {
		t.Fatalf("company B did not retain its independent review: %+v %v", gotB, err)
	}
	if err := svc.Set(ctx, companyA, actor, step, false, "", "", "integration"); err != nil {
		t.Fatal(err)
	}
	gotA, err = svc.List(ctx, companyA)
	if err != nil || len(gotA) != 0 {
		t.Fatalf("reopening review did not remove A's manual marking: %+v %v", gotA, err)
	}
	gotB, err = svc.List(ctx, companyB)
	if err != nil || len(gotB) != 1 {
		t.Fatalf("reopening A unexpectedly affected B: %+v %v", gotB, err)
	}

	for _, tenant := range []string{companyA, companyB} {
		var audits int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM audit_logs
			WHERE tenant_id=$1 AND action='setup.review.set'
		`, tenant).Scan(&audits); err != nil || audits < 1 {
			t.Fatalf("missing tenant audit for %s: %d %v", tenant, audits, err)
		}
		var invoices int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM invoices WHERE tenant_id=$1`,
			tenant).Scan(&invoices); err != nil || invoices != 0 {
			t.Fatalf("manual review created invoices: %d %v", invoices, err)
		}
	}
}
