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
	salesapp "github.com/example/sistemaemgo/internal/modules/sales/application"
	salesinfra "github.com/example/sistemaemgo/internal/modules/sales/infrastructure"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCashCloseWaitsForInFlightSaleAndIncludesIt(t *testing.T) {
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

	var tenantID, userID string
	if err := pool.QueryRow(ctx, `
		SELECT ut.tenant_id::text, ut.user_id::text
		FROM user_tenants ut
		JOIN users u ON u.id=ut.user_id
		WHERE u.active=true
		ORDER BY ut.created_at
		LIMIT 1
	`).Scan(&tenantID, &userID); err != nil {
		t.Fatalf("seeded tenant/user required: %v", err)
	}

	registerName := fmt.Sprintf("Concurrency CI %d", time.Now().UnixNano())
	var registerID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_registers(tenant_id, name, active)
		VALUES ($1,$2,true)
		RETURNING id::text
	`, tenantID, registerName).Scan(&registerID); err != nil {
		t.Fatalf("create register: %v", err)
	}

	var sessionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_sessions(tenant_id, cash_register_id, opened_by_user_id, opening_amount)
		VALUES ($1,$2,$3,100.00)
		RETURNING id::text
	`, tenantID, registerID, userID).Scan(&sessionID); err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM audit_logs WHERE resource_id=$1::uuid`, sessionID)
		_, _ = pool.Exec(bg, `DELETE FROM cash_session_reconciliations WHERE cash_session_id=$1`, sessionID)
		_, _ = pool.Exec(bg, `DELETE FROM payments WHERE sale_id IN (SELECT id FROM sales WHERE cash_session_id=$1)`, sessionID)
		_, _ = pool.Exec(bg, `DELETE FROM sales WHERE cash_session_id=$1`, sessionID)
		_, _ = pool.Exec(bg, `DELETE FROM cash_sessions WHERE id=$1`, sessionID)
		_, _ = pool.Exec(bg, `DELETE FROM cash_registers WHERE id=$1`, registerID)
	})

	cashRepo := salesinfra.NewCashRepo(pool)
	txSale, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin sale tx: %v", err)
	}
	defer func() { _ = txSale.Rollback(context.Background()) }()

	session, err := cashRepo.GetSession(ctx, txSale, tenantID, sessionID)
	if err != nil || session.Status != "open" {
		t.Fatalf("lock open cash session: session=%+v err=%v", session, err)
	}

	auditSvc := audit.New(pool, slog.Default())
	cashSvc := salesapp.NewCashService(db.NewPgxUnitOfWork(pool), cashRepo, nil, auditSvc, validator.New(), slog.Default())

	type closeResult struct {
		resultExpected platform.Money
		err            error
	}
	closed := make(chan closeResult, 1)
	go func() {
		result, err := cashSvc.CloseSession(
			context.Background(),
			tenantID,
			userID,
			sessionID,
			salesapp.CashCloseRequest{ClosingAmount: platform.NewMoneyCents(11000)},
		)
		closed <- closeResult{resultExpected: result.ExpectedCash, err: err}
	}()

	select {
	case got := <-closed:
		t.Fatalf("close returned before in-flight sale released cash lock: %+v", got)
	case <-time.After(200 * time.Millisecond):
		// Expected: close is blocked on cash_sessions FOR UPDATE.
	}

	var saleID string
	if err := txSale.QueryRow(ctx, `
		INSERT INTO sales(
			tenant_id, cash_session_id, status, subtotal, discount_value, total,
			profit_estimated, created_by_user_id
		)
		VALUES ($1,$2,'finalized',10.00,0,10.00,4.00,$3)
		RETURNING id::text
	`, tenantID, sessionID, userID).Scan(&saleID); err != nil {
		t.Fatalf("insert in-flight sale: %v", err)
	}
	if _, err := txSale.Exec(ctx, `
		INSERT INTO payments(tenant_id, sale_id, method, amount)
		VALUES ($1,$2,'cash',10.00)
	`, tenantID, saleID); err != nil {
		t.Fatalf("insert in-flight payment: %v", err)
	}
	if err := txSale.Commit(ctx); err != nil {
		t.Fatalf("commit in-flight sale: %v", err)
	}

	select {
	case got := <-closed:
		if got.err != nil {
			t.Fatalf("close after sale commit: %v", got.err)
		}
		if got.resultExpected.Cents() != 11000 {
			t.Fatalf("expected cash must include committed in-flight sale: got=%s want=110.00", got.resultExpected.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close did not resume after sale commit")
	}

	var status, expected string
	if err := pool.QueryRow(ctx, `
		SELECT status, expected_cash::text
		FROM cash_sessions
		WHERE id=$1
	`, sessionID).Scan(&status, &expected); err != nil {
		t.Fatalf("read closed session: %v", err)
	}
	if status != "closed" || expected != "110.00" {
		t.Fatalf("unexpected closed session status=%s expected_cash=%s", status, expected)
	}
}
