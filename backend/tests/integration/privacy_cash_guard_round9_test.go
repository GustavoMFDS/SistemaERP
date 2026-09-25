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

func TestPrivacyUserDeactivationCannotStrandOpenCash(t *testing.T) {
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

	tenantID := round7CreateTenant(t, ctx, pool, "Round9 Privacy Cash")
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
		INSERT INTO user_tenants(user_id, tenant_id)
		VALUES ($1,$2)
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
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_sessions(
			tenant_id, cash_register_id, opened_by_user_id, opening_amount, status
		)
		VALUES ($1,$2,$3,0,'open')
		RETURNING id::text
	`, tenantID, registerID, userID).Scan(&sessionID); err != nil {
		t.Fatalf("create open cash: %v", err)
	}

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM cash_sessions WHERE id=$1`, sessionID)
		_, _ = pool.Exec(bg, `DELETE FROM cash_registers WHERE id=$1`, registerID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id=$1`, userID)
		_, _ = pool.Exec(bg, `DELETE FROM companies WHERE id=$1`, tenantID)
	})

	privacyRepo := privacyinfra.NewRepo(pool)

	if err := privacyRepo.BlockSubject(ctx, tenantID, "user", userID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("block with open cash must conflict, got %v", err)
	}
	if err := privacyRepo.AnonymizeSubject(ctx, tenantID, "user", userID); !errors.Is(err, common.ErrConflict) {
		t.Fatalf("anonymize with open cash must conflict, got %v", err)
	}

	var active bool
	if err := pool.QueryRow(ctx, `
		SELECT active
		FROM user_tenants
		WHERE tenant_id=$1 AND user_id=$2
	`, tenantID, userID).Scan(&active); err != nil {
		t.Fatalf("read membership after conflicts: %v", err)
	}
	if !active {
		t.Fatal("open-cash privacy conflict must preserve active membership")
	}

	if _, err := pool.Exec(ctx, `
		UPDATE cash_sessions
		SET status='closed', closed_at=now(), closed_by_user_id=$2, closing_amount=0
		WHERE id=$1
	`, sessionID, userID); err != nil {
		t.Fatalf("close cash fixture: %v", err)
	}

	if err := privacyRepo.BlockSubject(ctx, tenantID, "user", userID); err != nil {
		t.Fatalf("block after cash close: %v", err)
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

	cashRepo := salesinfra.NewCashRepo(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin cash open check: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := cashRepo.OpenSession(
		ctx,
		tx,
		tenantID,
		registerID,
		userID,
		platform.NewMoneyCents(0),
		nil,
	); !errors.Is(err, common.ErrForbidden) {
		t.Fatalf("inactive membership must not open cash, got %v", err)
	}
}
