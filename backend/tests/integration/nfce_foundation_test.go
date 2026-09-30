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
	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	fiscinfra "github.com/example/sistemaemgo/internal/modules/fiscal/infrastructure"
	"github.com/example/sistemaemgo/internal/platform/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNFCeFoundation_TenantIsolationAndConstraints(t *testing.T) {
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

	var tenantA, actorA, productA string
	if err := pool.QueryRow(ctx, `
		SELECT utr.tenant_id::text, utr.user_id::text, p.id::text
		FROM user_tenant_roles utr
		JOIN roles r ON r.id=utr.role_id AND r.name='admin'
		JOIN products p ON p.tenant_id=utr.tenant_id
		ORDER BY p.created_at
		LIMIT 1
	`).Scan(&tenantA, &actorA, &productA); err != nil {
		t.Fatalf("seeded tenant/admin/product required: %v", err)
	}

	cnpj := fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000)
	var tenantB string
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies(legal_name, trade_name, cnpj)
		VALUES ('NFCe Tenant B', 'NFCe Tenant B', $1)
		RETURNING id::text
	`, cnpj).Scan(&tenantB); err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM fiscal_document_sequences WHERE tenant_id=$1`, tenantA)
		_, _ = pool.Exec(context.Background(), `DELETE FROM nfce_configs WHERE tenant_id=$1`, tenantA)
		_, _ = pool.Exec(context.Background(), `DELETE FROM companies WHERE id=$1`, tenantB)
	})

	repo := fiscinfra.NewFiscalRepo(pool)
	uow := db.NewPgxUnitOfWork(pool)

	cscID := "1"
	cscRef := "secret://integration/nfce/csc"
	certRef := "secret://integration/nfce/certificate"
	cfg := fisc.NFCeConfig{
		TenantID:             tenantA,
		Environment:          "homologation",
		Series:               1,
		CSCID:                &cscID,
		CSCSecretRef:         &cscRef,
		CertificateSecretRef: &certRef,
	}

	tx, err := uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin config tx: %v", err)
	}
	if err := repo.UpsertNFCeConfig(ctx, tx, tenantA, actorA, cfg); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("upsert tenant A config: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tenant A config: %v", err)
	}

	got, err := repo.GetNFCeConfig(ctx, tenantA)
	if err != nil {
		t.Fatalf("get tenant A config: %v", err)
	}
	if got.Enabled {
		t.Fatal("preparation config must remain disabled")
	}
	if !got.CSCReferenceConfigured || !got.CertificateReferenceConfigured {
		t.Fatalf("secret-reference status missing: %+v", got)
	}
	if _, err := repo.GetNFCeConfig(ctx, tenantB); err == nil {
		t.Fatal("tenant B unexpectedly read tenant A NFC-e config")
	}

	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin cross-tenant actor tx: %v", err)
	}
	crossErr := repo.UpsertNFCeConfig(ctx, tx, tenantB, actorA, fisc.NFCeConfig{
		TenantID:    tenantB,
		Environment: "homologation",
		Series:      1,
	})
	_ = tx.Rollback(ctx)
	if crossErr == nil {
		t.Fatal("database unexpectedly accepted NFC-e config actor from another tenant")
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO nfce_configs(tenant_id, environment, series)
		VALUES ($1, 'homologation', 890)
	`, tenantB); err == nil {
		t.Fatal("database unexpectedly accepted NFC-e series 890")
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO nfce_configs(tenant_id, enabled, environment, series)
		VALUES ($1, true, 'homologation', 1)
	`, tenantB); err == nil {
		t.Fatal("database unexpectedly enabled NFC-e without CSC/certificate references")
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO fiscal_document_sequences(tenant_id, model, series, next_number)
		VALUES ($1, 55, 1, 1)
	`, tenantB); err == nil {
		t.Fatal("database unexpectedly accepted fiscal model 55 in NFC-e sequence")
	}

	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin first sequence tx: %v", err)
	}
	number, err := repo.ReserveNextNFCeNumber(ctx, tx, tenantA, 889)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("reserve first NFC-e number: %v", err)
	}
	if number != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("first NFC-e number=%d, want 1", number)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit first sequence tx: %v", err)
	}

	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin rollback sequence tx: %v", err)
	}
	number, err = repo.ReserveNextNFCeNumber(ctx, tx, tenantA, 889)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("reserve rollback NFC-e number: %v", err)
	}
	if number != 2 {
		_ = tx.Rollback(ctx)
		t.Fatalf("second NFC-e number=%d, want 2", number)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback sequence tx: %v", err)
	}

	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin reused sequence tx: %v", err)
	}
	number, err = repo.ReserveNextNFCeNumber(ctx, tx, tenantA, 889)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("reserve reused NFC-e number: %v", err)
	}
	if number != 2 {
		_ = tx.Rollback(ctx)
		t.Fatalf("number after rollback=%d, want 2", number)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit reused sequence tx: %v", err)
	}

	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tenant B sequence tx: %v", err)
	}
	number, err = repo.ReserveNextNFCeNumber(ctx, tx, tenantB, 889)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("reserve tenant B NFC-e number: %v", err)
	}
	if number != 1 {
		_ = tx.Rollback(ctx)
		t.Fatalf("tenant B first NFC-e number=%d, want 1", number)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tenant B sequence tx: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO fiscal_document_sequences(tenant_id, model, series, next_number)
		VALUES ($1, 65, 888, 1000000000)
	`, tenantB); err != nil {
		t.Fatalf("seed exhausted sequence sentinel: %v", err)
	}
	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin exhausted sequence tx: %v", err)
	}
	_, reserveErr := repo.ReserveNextNFCeNumber(ctx, tx, tenantB, 888)
	_ = tx.Rollback(ctx)
	if !errors.Is(reserveErr, common.ErrFiscalSequenceExhausted) {
		t.Fatalf("exhausted sequence error=%v, want ErrFiscalSequenceExhausted", reserveErr)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE products SET ncm='1234567A' WHERE tenant_id=$1 AND id=$2
	`, tenantA, productA); err == nil {
		t.Fatal("database unexpectedly accepted invalid product NCM")
	}
}
