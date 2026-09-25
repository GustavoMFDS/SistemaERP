//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/modules/common"
	privacyinfra "github.com/example/sistemaemgo/internal/modules/privacy/infrastructure"
	salesinfra "github.com/example/sistemaemgo/internal/modules/sales/infrastructure"
	"github.com/example/sistemaemgo/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPrivacyMembershipDeactivationAndCashOpeningCannotOrphanSession(t *testing.T) {
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

	tenantID := round7CreateTenant(t, ctx, pool, "Round9 Privacy Cash Tenant")
	email := fmt.Sprintf("round9-privacy-cash-%d@example.test", time.Now().UnixNano())

	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users(email, name, password_hash, active)
		VALUES ($1,'Round9 Privacy Cash User','not-used',true)
		RETURNING id::text
	`, email).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_tenants(user_id, tenant_id, active)
		VALUES ($1,$2,true)
	`, userID, tenantID); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	var registerID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_registers(tenant_id, name, active)
		VALUES ($1,$2,true)
		RETURNING id::text
	`, tenantID, fmt.Sprintf("Round9 Privacy Cash %d", time.Now().UnixNano())).Scan(&registerID); err != nil {
		t.Fatalf("create register: %v", err)
	}

	var sessionID string
	t.Cleanup(func() {
		bg := context.Background()
		if sessionID != "" {
			_, _ = pool.Exec(bg, `DELETE FROM cash_sessions WHERE id=$1`, sessionID)
		}
		_, _ = pool.Exec(bg, `DELETE FROM cash_registers WHERE id=$1`, registerID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id=$1`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM companies WHERE id=$1`, tenantID)
	})

	cashRepo := salesinfra.NewCashRepo(pool)
	privacyRepo := privacyinfra.NewRepo(pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin cash open: %v", err)
	}
	sessionID, err = cashRepo.OpenSession(
		ctx,
		tx,
		tenantID,
		registerID,
		userID,
		platform.NewMoneyCents(0),
		nil,
	)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("open cash: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit cash open: %v", err)
	}

	if err := privacyRepo.AnonymizeSubject(ctx, tenantID, "user", userID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("open cash must block user anonymization, got %v", err)
	}
	if err := privacyRepo.BlockSubject(ctx, tenantID, "user", userID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("open cash must block membership deactivation, got %v", err)
	}

	var active bool
	if err := pool.QueryRow(ctx, `
		SELECT active
		FROM user_tenants
		WHERE tenant_id=$1 AND user_id=$2
	`, tenantID, userID).Scan(&active); err != nil {
		t.Fatalf("read membership after blocked privacy action: %v", err)
	}
	if !active {
		t.Fatal("membership was deactivated while user still had an open cash session")
	}

	if _, err := pool.Exec(ctx, `
		UPDATE cash_sessions
		SET status='closed', closed_at=now(), closed_by_user_id=$2,
		    closing_amount=0, expected_cash=0, closing_difference=0
		WHERE id=$1
	`, sessionID, userID); err != nil {
		t.Fatalf("close fixture cash: %v", err)
	}

	if err := privacyRepo.BlockSubject(ctx, tenantID, "user", userID); err != nil {
		t.Fatalf("block membership after cash close: %v", err)
	}

	if err := pool.QueryRow(ctx, `
		SELECT active
		FROM user_tenants
		WHERE tenant_id=$1 AND user_id=$2
	`, tenantID, userID).Scan(&active); err != nil {
		t.Fatalf("read blocked membership: %v", err)
	}
	if active {
		t.Fatal("membership should be inactive after successful block")
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin rejected cash open: %v", err)
	}
	_, err = cashRepo.OpenSession(
		ctx,
		tx,
		tenantID,
		registerID,
		userID,
		platform.NewMoneyCents(0),
		nil,
	)
	if !errors.Is(err, common.ErrForbidden) {
		_ = tx.Rollback(ctx)
		t.Fatalf("inactive membership must reject new cash open with ErrForbidden, got %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback rejected cash open: %v", err)
	}

	var openCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM cash_sessions
		WHERE tenant_id=$1 AND opened_by_user_id=$2 AND status='open'
	`, tenantID, userID).Scan(&openCount); err != nil {
		t.Fatalf("count orphan open cash: %v", err)
	}
	if openCount != 0 {
		t.Fatalf("inactive membership ended with %d open cash session(s)", openCount)
	}
}
