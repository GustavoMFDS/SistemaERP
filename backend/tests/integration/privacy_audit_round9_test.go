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

func TestPrivacyAnonymizationStatusAndAuditCommitAtomically(t *testing.T) {
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

	tenantB := round7CreateTenant(t, ctx, pool, "Round9 Privacy Audit Tenant B")
	foreignEmail := fmt.Sprintf("round9-privacy-audit-%d@example.test", time.Now().UnixNano())
	var actorB string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users(email, name, password_hash, active)
		VALUES ($1,'Round9 Privacy Foreign Actor','not-used',true)
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

	customerEmail := fmt.Sprintf("round9-customer-%d@example.test", time.Now().UnixNano())
	customerDocument := fmt.Sprintf("R9DOC-%d", time.Now().UnixNano())
	var customerID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO customers(tenant_id, name, document, email, phone)
		VALUES ($1,'Round9 Privacy Customer',$2,$3,'34999999999')
		RETURNING id::text
	`, tenantA, customerDocument, customerEmail).Scan(&customerID); err != nil {
		t.Fatalf("create customer: %v", err)
	}

	var requestID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO data_subject_requests(
			tenant_id, subject_type, subject_id, request_type, status, created_by_user_id
		)
		VALUES ($1,'customer',$2,'anonymization','in_progress',$3)
		RETURNING id::text
	`, tenantA, customerID, actorA).Scan(&requestID); err != nil {
		t.Fatalf("create DSR: %v", err)
	}

	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM audit_logs WHERE resource_id=$1::uuid`, requestID)
		_, _ = pool.Exec(bg, `DELETE FROM data_subject_requests WHERE id=$1`, requestID)
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

	if err := svc.AnonymizeSubject(ctx, tenantA, actorB, requestID); err == nil {
		t.Fatal("foreign audit actor must make anonymization transaction fail")
	}

	var name, document, email, phone, status string
	var resolvedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT c.name, c.document, c.email::text, c.phone, d.status, d.resolved_at
		FROM customers c
		JOIN data_subject_requests d
		  ON d.tenant_id=c.tenant_id
		 AND d.subject_id=c.id
		WHERE c.id=$1 AND d.id=$2
	`, customerID, requestID).Scan(&name, &document, &email, &phone, &status, &resolvedAt); err != nil {
		t.Fatalf("read rolled-back privacy state: %v", err)
	}
	if name != "Round9 Privacy Customer" ||
		document != customerDocument ||
		email != customerEmail ||
		phone != "34999999999" ||
		status != "in_progress" ||
		resolvedAt != nil {
		t.Fatalf(
			"privacy mutation committed without audit: name=%q document=%q email=%q phone=%q status=%q resolved=%v",
			name, document, email, phone, status, resolvedAt,
		)
	}

	var auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE tenant_id=$1
		  AND resource_id=$2
		  AND action='privacy.subject.anonymize'
	`, tenantA, requestID).Scan(&auditCount); err != nil {
		t.Fatalf("count rolled-back privacy audit: %v", err)
	}
	if auditCount != 0 {
		t.Fatalf("failed privacy transaction left %d audit row(s)", auditCount)
	}

	if err := svc.AnonymizeSubject(ctx, tenantA, actorA, requestID); err != nil {
		t.Fatalf("valid anonymization: %v", err)
	}

	var documentAfter, emailAfter, phoneAfter *string
	if err := pool.QueryRow(ctx, `
		SELECT c.name, c.document, c.email::text, c.phone, d.status, d.resolved_at
		FROM customers c
		JOIN data_subject_requests d
		  ON d.tenant_id=c.tenant_id
		 AND d.subject_id=c.id
		WHERE c.id=$1 AND d.id=$2
	`, customerID, requestID).Scan(&name, &documentAfter, &emailAfter, &phoneAfter, &status, &resolvedAt); err != nil {
		t.Fatalf("read committed privacy state: %v", err)
	}
	if name != "Titular anonimizado" ||
		documentAfter != nil ||
		emailAfter != nil ||
		phoneAfter != nil ||
		status != "completed" ||
		resolvedAt == nil {
		t.Fatalf(
			"unexpected committed privacy state: name=%q document=%v email=%v phone=%v status=%q resolved=%v",
			name, documentAfter, emailAfter, phoneAfter, status, resolvedAt,
		)
	}

	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM audit_logs
		WHERE tenant_id=$1
		  AND actor_user_id=$2
		  AND resource_id=$3
		  AND action='privacy.subject.anonymize'
	`, tenantA, actorA, requestID).Scan(&auditCount); err != nil {
		t.Fatalf("count committed privacy audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("committed anonymization audit count=%d, want 1", auditCount)
	}
}
