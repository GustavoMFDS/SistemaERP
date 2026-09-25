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
	privacyapp "github.com/example/sistemaemgo/internal/modules/privacy/application"
	privacyinfra "github.com/example/sistemaemgo/internal/modules/privacy/infrastructure"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPrivacyConsentCreateAndRevokeAuditAtomically(t *testing.T) {
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

	var tenantA, actorA string
	if err := pool.QueryRow(ctx, `
		SELECT ut.tenant_id::text, ut.user_id::text
		FROM user_tenants ut
		JOIN users u ON u.id=ut.user_id
		WHERE ut.active=true AND u.active=true
		ORDER BY ut.created_at
		LIMIT 1
	`).Scan(&tenantA, &actorA); err != nil {
		t.Fatalf("seeded tenant/actor required: %v", err)
	}

	tenantB := round7CreateTenant(t, ctx, pool, "Round9 Consent Audit Tenant B")
	foreignEmail := fmt.Sprintf("round9-consent-audit-%d@example.test", time.Now().UnixNano())
	var actorB string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users(email, name, password_hash, active)
		VALUES ($1,'Round9 Consent Foreign Actor','not-used',true)
		RETURNING id::text
	`, foreignEmail).Scan(&actorB); err != nil {
		t.Fatalf("create foreign actor: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_tenants(user_id, tenant_id, active)
		VALUES ($1,$2,true)
	`, actorB, tenantB); err != nil {
		t.Fatalf("create foreign membership: %v", err)
	}

	customerEmail := fmt.Sprintf("round9-consent-customer-%d@example.test", time.Now().UnixNano())
	var customerID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO customers(tenant_id, name, email)
		VALUES ($1,'Round9 Consent Customer',$2)
		RETURNING id::text
	`, tenantA, customerEmail).Scan(&customerID); err != nil {
		t.Fatalf("create customer: %v", err)
	}

	purpose := fmt.Sprintf("round9-consent-%d", time.Now().UnixNano())
	var consentID string
	t.Cleanup(func() {
		bg := context.Background()
		if consentID != "" {
			_, _ = pool.Exec(bg, `DELETE FROM audit_logs WHERE resource_id=$1::uuid`, consentID)
			_, _ = pool.Exec(bg, `DELETE FROM consent_records WHERE id=$1`, consentID)
		}
		_, _ = pool.Exec(bg, `DELETE FROM consent_records WHERE tenant_id=$1 AND purpose=$2`, tenantA, purpose)
		_, _ = pool.Exec(bg, `DELETE FROM customers WHERE id=$1`, customerID)
		_, _ = pool.Exec(bg, `DELETE FROM users WHERE id=$1`, actorB)
		_, _ = pool.Exec(bg, `DELETE FROM companies WHERE id=$1`, tenantB)
	})

	repo := privacyinfra.NewRepo(pool)
	svc := privacyapp.NewService(
		db.NewPgxUnitOfWork(pool),
		repo,
		audit.New(pool, slog.Default()),
	)
	req := privacyapp.ConsentCreateRequest{
		SubjectType:        "customer",
		SubjectID:          &customerID,
		Purpose:            purpose,
		ConsentTextVersion: "round9-v1",
		Source:             "integration",
	}

	if _, err := svc.RecordConsent(ctx, tenantA, actorB, "round9-invalid-actor", req); err == nil {
		t.Fatal("foreign audit actor must make consent create transaction fail")
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM consent_records
		WHERE tenant_id=$1 AND subject_id=$2 AND purpose=$3
	`, tenantA, customerID, purpose).Scan(&count); err != nil {
		t.Fatalf("count rolled-back consent: %v", err)
	}
	if count != 0 {
		t.Fatalf("consent committed without valid audit actor: count=%d", count)
	}

	consentID, err = svc.RecordConsent(ctx, tenantA, actorA, "round9-valid-actor", req)
	if err != nil {
		t.Fatalf("valid consent create: %v", err)
	}

	var createAudits int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE tenant_id=$1
		  AND actor_user_id=$2
		  AND resource_id=$3
		  AND action='privacy.consent.create'
	`, tenantA, actorA, consentID).Scan(&createAudits); err != nil {
		t.Fatalf("count consent create audit: %v", err)
	}
	if createAudits != 1 {
		t.Fatalf("consent create audit count=%d, want 1", createAudits)
	}

	if err := svc.RevokeConsent(ctx, tenantA, actorB, consentID); err == nil {
		t.Fatal("foreign audit actor must make consent revoke transaction fail")
	}

	var withdrawnAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT withdrawn_at
		FROM consent_records
		WHERE tenant_id=$1 AND id=$2
	`, tenantA, consentID).Scan(&withdrawnAt); err != nil {
		t.Fatalf("read rolled-back consent revoke: %v", err)
	}
	if withdrawnAt != nil {
		t.Fatalf("consent revoke committed without audit: withdrawn_at=%v", withdrawnAt)
	}

	if err := svc.RevokeConsent(ctx, tenantA, actorA, consentID); err != nil {
		t.Fatalf("valid consent revoke: %v", err)
	}

	var revokeAudits int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE tenant_id=$1
		  AND actor_user_id=$2
		  AND resource_id=$3
		  AND action='privacy.consent.revoke'
	`, tenantA, actorA, consentID).Scan(&revokeAudits); err != nil {
		t.Fatalf("count consent revoke audit: %v", err)
	}
	if revokeAudits != 1 {
		t.Fatalf("consent revoke audit count=%d, want 1", revokeAudits)
	}
}
