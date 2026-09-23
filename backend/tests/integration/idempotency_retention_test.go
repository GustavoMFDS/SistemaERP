//go:build integration

package integration_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/app"
	"github.com/example/sistemaemgo/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAppStartupCleansOnlyExpiredIdempotencyKeys(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	var tenantID, userID string
	if err := pool.QueryRow(ctx, `
		SELECT c.id::text, u.id::text
		FROM companies c
		JOIN user_tenants ut ON ut.tenant_id=c.id
		JOIN users u ON u.id=ut.user_id
		WHERE u.email='admin@sistema.local'
		LIMIT 1
	`).Scan(&tenantID, &userID); err != nil {
		t.Fatalf("seeded tenant/admin required: %v", err)
	}

	var registerID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_registers(tenant_id, name, active)
		VALUES ($1, 'Idempotency Retention CI', true)
		ON CONFLICT (tenant_id, name) DO UPDATE SET active=EXCLUDED.active
		RETURNING id::text
	`, tenantID).Scan(&registerID); err != nil {
		t.Fatalf("cash register: %v", err)
	}

	var sessionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO cash_sessions(
			tenant_id, cash_register_id, opened_by_user_id, opening_amount, status
		)
		VALUES ($1,$2,$3,0,'closed')
		RETURNING id::text
	`, tenantID, registerID, userID).Scan(&sessionID); err != nil {
		t.Fatalf("cash session: %v", err)
	}

	var saleID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO sales(
			tenant_id, cash_session_id, status, subtotal, discount_value, total,
			profit_estimated, created_by_user_id
		)
		VALUES ($1,$2,'finalized',1,0,1,0,$3)
		RETURNING id::text
	`, tenantID, sessionID, userID).Scan(&saleID); err != nil {
		t.Fatalf("sale fixture: %v", err)
	}

	oldKey := "retention-old-" + saleID
	freshKey := "retention-fresh-" + saleID
	if _, err := pool.Exec(ctx, `
		INSERT INTO idempotency_keys(
			tenant_id, operation, idem_key, request_hash, sale_id, total, created_at
		)
		VALUES
			($1,'sales.create_and_finalize',$2,'old-hash',$4,1,now()-interval '31 days'),
			($1,'sales.create_and_finalize',$3,'fresh-hash',$4,1,now()-interval '1 day')
	`, tenantID, oldKey, freshKey, saleID); err != nil {
		t.Fatalf("seed idempotency keys: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM idempotency_keys WHERE idem_key IN ($1,$2)`, oldKey, freshKey)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM sales WHERE id=$1`, saleID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM cash_sessions WHERE id=$1`, sessionID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM cash_registers WHERE id=$1`, registerID)
	})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := app.New(ctx, config.Config{
		Env:          "test",
		DatabaseURL:  databaseURL,
		DisableRedis: true,
		JWTSecret:    "integration-idempotency-retention-secret-32-chars",
		JWTIssuer:    "sistemaemgo-integration",
		FiscalProvider: "mvp",
	}, logger)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	defer application.Close()

	var oldCount, freshCount int
	if err := pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE idem_key=$1),
			count(*) FILTER (WHERE idem_key=$2)
		FROM idempotency_keys
		WHERE tenant_id=$3
	`, oldKey, freshKey, tenantID).Scan(&oldCount, &freshCount); err != nil {
		t.Fatalf("verify retention: %v", err)
	}
	if oldCount != 0 {
		t.Fatalf("expired idempotency key survived startup cleanup")
	}
	if freshCount != 1 {
		t.Fatalf("fresh idempotency key was deleted: count=%d", freshCount)
	}
}
