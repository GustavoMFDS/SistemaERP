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
	"github.com/example/sistemaemgo/internal/modules/customers"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCustomerDirectoryIsTenantScopedAndAudited(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	var actor string
	if err := db.QueryRow(ctx, `SELECT id::text FROM users WHERE email='admin@sistema.local'`).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	var tenants []string
	for i := 0; i < 2; i++ {
		var tenantID string
		cnpj := fmt.Sprintf("%014d", (time.Now().UnixNano()+int64(i))%100000000000000)
		if err := db.QueryRow(ctx, `
			INSERT INTO companies(legal_name,cnpj) VALUES('Customer Directory Test',$1)
			RETURNING id::text
		`, cnpj).Scan(&tenantID); err != nil {
			t.Fatal(err)
		}
		tenants = append(tenants, tenantID)
		if _, err := db.Exec(ctx, `
			INSERT INTO user_tenants(user_id,tenant_id,active)
			VALUES($1,$2,true)
		`, actor, tenantID); err != nil {
			t.Fatal(err)
		}
	}
	tenantA, tenantB := tenants[0], tenants[1]
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		for _, tenantID := range tenants {
			_, _ = db.Exec(cleanup, `DELETE FROM audit_logs WHERE tenant_id=$1`, tenantID)
			_, _ = db.Exec(cleanup, `DELETE FROM customers WHERE tenant_id=$1`, tenantID)
			_, _ = db.Exec(cleanup, `DELETE FROM user_tenants WHERE tenant_id=$1`, tenantID)
			_, _ = db.Exec(cleanup, `DELETE FROM companies WHERE id=$1`, tenantID)
		}
	}()
	svc := customers.New(db, audit.New(db, slog.New(slog.NewTextHandler(io.Discard, nil))))
	email := fmt.Sprintf("cliente-%d@example.test", time.Now().UnixNano())
	created, err := svc.Create(ctx, tenantA, actor, customers.CustomerInput{
		Name: "Cliente Exemplo", Email: &email,
	}, "", "", "")
	if err != nil || created.ID == "" || created.Name != "Cliente Exemplo" {
		t.Fatalf("create customer: %+v err=%v", created, err)
	}
	own, err := svc.List(ctx, tenantA, "Cliente", 20, 0)
	if err != nil || len(own.Items) != 1 || own.Total != 1 || own.Items[0].ID != created.ID {
		t.Fatalf("company A missing customer: %+v err=%v", own, err)
	}
	other, err := svc.List(ctx, tenantB, "Cliente", 20, 0)
	if err != nil || len(other.Items) != 0 || other.Total != 0 {
		t.Fatalf("cross-company customer leak: %+v err=%v", other, err)
	}
	_, err = svc.Update(ctx, tenantB, actor, created.ID, customers.CustomerInput{Name: "Alterado"}, "", "", "")
	if !errors.Is(err, common.ErrNotFound) {
		t.Fatalf("company B changed A customer: %v", err)
	}
	updated, err := svc.Update(ctx, tenantA, actor, created.ID, customers.CustomerInput{
		Name: "Cliente Revisado", Phone: strPtr("11990001122"),
	}, "", "", "")
	if err != nil || updated.Name != "Cliente Revisado" {
		t.Fatalf("customer update: %+v err=%v", updated, err)
	}
	var audits int
	if err := db.QueryRow(ctx, `
		SELECT count(*) FROM audit_logs
		WHERE tenant_id=$1 AND action IN ('customer.create','customer.update')
	`, tenantA).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("customer writes must be audited: %d %v", audits, err)
	}
	if _, err := svc.List(ctx, tenantA, "", 51, 0); !errors.Is(err, common.ErrValidation) {
		t.Fatalf("unbounded list accepted: %v", err)
	}
}

func strPtr(value string) *string { return &value }
