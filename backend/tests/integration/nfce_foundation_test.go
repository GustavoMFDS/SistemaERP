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

func unusedNFCeSeries(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tenantID string, consecutive int) int {
	t.Helper()
	const maxSeries = 889
	if consecutive < 1 || consecutive > maxSeries {
		t.Fatalf("invalid consecutive NFC-e series count: %d", consecutive)
	}

	rows, err := pool.Query(ctx, `
		SELECT series FROM fiscal_document_sequences
		WHERE tenant_id=$1 AND model=65
	`, tenantID)
	if err != nil {
		t.Fatalf("list previously used NFC-e series: %v", err)
	}
	defer rows.Close()

	used := make(map[int]struct{})
	for rows.Next() {
		var series int
		if err := rows.Scan(&series); err != nil {
			t.Fatalf("scan previously used NFC-e series: %v", err)
		}
		used[series] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate previously used NFC-e series: %v", err)
	}

	for first := 1; first+consecutive-1 <= maxSeries; first++ {
		available := true
		for offset := 0; offset < consecutive; offset++ {
			if _, exists := used[first+offset]; exists {
				available = false
				break
			}
		}
		if available {
			return first
		}
	}
	t.Fatalf("no %d consecutive unused NFC-e series for tenant %s", consecutive, tenantID)
	return 0
}

func testNFCeCNPJCheckDigit(digits []byte, weights []int) byte {
	sum := 0
	for i, weight := range weights {
		sum += int(digits[i]-'0') * weight
	}
	remainder := sum % 11
	if remainder < 2 {
		return '0'
	}
	return byte('0' + 11 - remainder)
}

func createNFCeTestTenant(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	name string,
	withAdmin bool,
) (string, string) {
	t.Helper()

	var actorUserID string
	if withAdmin {
		if err := pool.QueryRow(ctx, `
			SELECT utr.user_id::text
			FROM user_tenant_roles utr
			JOIN roles r ON r.id=utr.role_id AND r.name='admin'
			ORDER BY utr.user_id
			LIMIT 1
		`).Scan(&actorUserID); err != nil {
			t.Fatalf("seeded admin required for isolated NFC-e tenant: %v", err)
		}
	}

	base := fmt.Sprintf("%012d", time.Now().UnixNano()%1_000_000_000_000)
	digits := []byte(base)
	firstWeights := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	digits = append(digits, testNFCeCNPJCheckDigit(digits, firstWeights))
	secondWeights := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	digits = append(digits, testNFCeCNPJCheckDigit(digits, secondWeights))

	var tenantID string
	legalName := fmt.Sprintf("%s %s", name, base)
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies(legal_name, trade_name, cnpj)
		VALUES ($1, $1, $2)
		RETURNING id::text
	`, legalName, string(digits)).Scan(&tenantID); err != nil {
		t.Fatalf("create isolated NFC-e tenant: %v", err)
	}

	if withAdmin {
		if _, err := pool.Exec(ctx, `
			INSERT INTO user_tenants(user_id, tenant_id, active)
			VALUES ($1, $2, true)
		`, actorUserID, tenantID); err != nil {
			t.Fatalf("associate seeded admin with isolated NFC-e tenant: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO user_tenant_roles(user_id, tenant_id, role_id)
			SELECT $1, $2, id FROM roles WHERE name='admin'
		`, actorUserID, tenantID); err != nil {
			t.Fatalf("grant admin role to isolated NFC-e tenant: %v", err)
		}
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := pool.Exec(cleanupCtx,
				`DELETE FROM user_tenant_roles WHERE tenant_id=$1 AND user_id=$2`,
				tenantID, actorUserID,
			); err != nil {
				t.Errorf("remove isolated NFC-e tenant role: %v", err)
			}
			if _, err := pool.Exec(cleanupCtx,
				`DELETE FROM user_tenants WHERE tenant_id=$1 AND user_id=$2`,
				tenantID, actorUserID,
			); err != nil {
				t.Errorf("remove isolated NFC-e tenant membership: %v", err)
			}
		})
	}
	return tenantID, actorUserID
}

func TestNFCeFoundation_TenantIsolationAndConstraints(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	// This scenario COMMITs fiscal sequences. It must execute only against a
	// newly created, disposable database, never a pilot or company database.
	if os.Getenv("TEST_FISCAL_DISPOSABLE_DB") != "1" {
		t.Skip("NFC-e sequence test requires an isolated disposable database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}

	t.Cleanup(pool.Close)

	tenantA, actorA := createNFCeTestTenant(t, ctx, pool, "NFCe Foundation A", true)
	var productA string
	if err := pool.QueryRow(ctx, `
		INSERT INTO products(
			tenant_id, sku, name, unit, cost_price, price_cash,
			min_stock, active, ncm
		)
		VALUES ($1, $2, 'NFC-e foundation product', 'UN', 1, 2, 0, true, '61091000')
		RETURNING id::text
	`, tenantA, fmt.Sprintf("NFCe-FOUNDATION-%d", time.Now().UnixNano())).Scan(&productA); err != nil {
		t.Fatalf("create isolated foundation product: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(),
			`DELETE FROM products WHERE tenant_id=$1 AND id=$2`, tenantA, productA,
		); err != nil {
			t.Errorf("remove isolated foundation product: %v", err)
		}
	})

	tenantASeries := unusedNFCeSeries(t, ctx, pool, tenantA, 1)
	tenantB, _ := createNFCeTestTenant(t, ctx, pool, "NFCe Foundation B", false)

	// Intentionally do not delete fiscal_document_sequences or nfce_configs:
	// those rows may have protected audit history. The dedicated test runner
	// destroys this database only after the test process finishes.

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
	number, err := repo.ReserveNextNFCeNumber(ctx, tx, tenantA, tenantASeries)
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
	number, err = repo.ReserveNextNFCeNumber(ctx, tx, tenantA, tenantASeries)
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
	number, err = repo.ReserveNextNFCeNumber(ctx, tx, tenantA, tenantASeries)
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
		VALUES ($1, 65, 887, 999999999)
	`, tenantB); err != nil {
		t.Fatalf("seed last reservable NFC-e number: %v", err)
	}
	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin last-number sequence tx: %v", err)
	}
	number, err = repo.ReserveNextNFCeNumber(ctx, tx, tenantB, 887)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("reserve last NFC-e number: %v", err)
	}
	if number != 999999999 {
		_ = tx.Rollback(ctx)
		t.Fatalf("last NFC-e number=%d, want 999999999", number)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit last-number sequence tx: %v", err)
	}
	tx, err = uow.Begin(ctx)
	if err != nil {
		t.Fatalf("begin post-last sequence tx: %v", err)
	}
	_, reserveErr := repo.ReserveNextNFCeNumber(ctx, tx, tenantB, 887)
	_ = tx.Rollback(ctx)
	if !errors.Is(reserveErr, common.ErrFiscalSequenceExhausted) {
		t.Fatalf("post-last sequence error=%v, want ErrFiscalSequenceExhausted", reserveErr)
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
	_, reserveErr = repo.ReserveNextNFCeNumber(ctx, tx, tenantB, 888)
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
