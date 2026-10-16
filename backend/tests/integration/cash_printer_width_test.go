//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Only executes with an explicitly isolated disposable PostgreSQL database.
// Confirms preferences cannot be shared across independent store CNPJs.
func TestCashRegisterDANFEWidthTenantIsolationAndConstraints(t *testing.T) {
	if os.Getenv("TEST_FISCAL_DISPOSABLE_DB") != "1" || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("requires disposable PostgreSQL with migration 0048")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tenants := make([]string, 0, 2)
	defer func() {
		for _, tenant := range tenants {
			_, _ = pool.Exec(context.Background(), "DELETE FROM companies WHERE id=$1", tenant)
		}
	}()
	for i := 0; i < 2; i++ {
		var tenant string
		cnpj := fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000+int64(i))
		err := pool.QueryRow(ctx, `
			INSERT INTO companies(legal_name,cnpj)
			VALUES($1,$2) RETURNING id::text
		`, "Printer Test "+uuid.NewString(), cnpj).Scan(&tenant)
		if err != nil {
			t.Fatal(err)
		}
		tenants = append(tenants, tenant)
	}
	registers := make([]string, 0, 2)
	for _, tenant := range tenants {
		var register string
		err := pool.QueryRow(ctx, `
			INSERT INTO cash_registers(tenant_id,name,active)
			VALUES($1,$2,true) RETURNING id::text
		`, tenant, "Caixa "+uuid.NewString()).Scan(&register)
		if err != nil {
			t.Fatal(err)
		}
		registers = append(registers, register)
	}
	readWidth := func(tenant, register string) int {
		t.Helper()
		var width int
		err := pool.QueryRow(ctx, `
			SELECT danfe_paper_width_mm FROM cash_registers
			WHERE tenant_id=$1 AND id=$2
		`, tenant, register).Scan(&width)
		if err != nil {
			t.Fatal(err)
		}
		return width
	}
	for i := range registers {
		if got := readWidth(tenants[i], registers[i]); got != 80 {
			t.Fatalf("default paper width=%d, want 80", got)
		}
	}
	changed, err := pool.Exec(ctx, `
		UPDATE cash_registers SET danfe_paper_width_mm=58
		WHERE tenant_id=$1 AND id=$2
	`, tenants[1], registers[0])
	if err != nil || changed.RowsAffected() != 0 {
		t.Fatalf("cross-tenant print preference update allowed: %+v %v", changed, err)
	}
	changed, err = pool.Exec(ctx, `
		UPDATE cash_registers SET danfe_paper_width_mm=58
		WHERE tenant_id=$1 AND id=$2
	`, tenants[0], registers[0])
	if err != nil || changed.RowsAffected() != 1 {
		t.Fatalf("legitimate print preference update failed: %+v %v", changed, err)
	}
	if got := readWidth(tenants[0], registers[0]); got != 58 {
		t.Fatalf("58mm setting not persisted: %d", got)
	}
	if got := readWidth(tenants[1], registers[1]); got != 80 {
		t.Fatalf("register/tenant setting leaked: %d", got)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cash_registers SET danfe_paper_width_mm=60
		WHERE tenant_id=$1 AND id=$2
	`, tenants[0], registers[0]); err == nil {
		t.Fatal("unsupported paper roll size was accepted by SQL CHECK")
	}
	if got := readWidth(tenants[0], registers[0]); got != 58 {
		t.Fatalf("invalid update changed persisted print width: %d", got)
	}
}
