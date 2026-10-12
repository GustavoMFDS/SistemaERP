//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	fiscalapp "github.com/example/sistemaemgo/internal/modules/fiscal/application"
	fiscalinfra "github.com/example/sistemaemgo/internal/modules/fiscal/infrastructure"
	"github.com/jackc/pgx/v5/pgxpool"
)

// On a disposable database only: checks that readiness and the production
// predicate cannot mistake a shaped but invalid issuer for verified data.
func TestNFCeReadinessReportsInvalidIssuerAndMGCode(t *testing.T) {
	if os.Getenv("TEST_FISCAL_DISPOSABLE_DB") != "1" || os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("explicit disposable database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	tenantID := ""
	defer func() {
		if tenantID != "" {
			_, _ = pool.Exec(context.Background(), "DELETE FROM companies WHERE id=$1", tenantID)
		}
	}()
	cnpj := fmt.Sprintf("%014d", time.Now().UnixNano()%100000000000000)
	if err := pool.QueryRow(ctx, `
		INSERT INTO companies(legal_name,cnpj,ie,crt,
			address_street,address_number,address_neighborhood,address_city,
			address_city_code,address_state,address_zip)
		VALUES('Mercado Fiscal Teste',$1,'0012345678901','3',
			'Rua Um','1','Centro','Belo Horizonte','3106200','MG','30130010')
		RETURNING id::text
	`, cnpj).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	svc := fiscalapp.NewFiscalService(nil, fiscalinfra.NewFiscalRepo(pool), nil, nil, nil, nil, nil)
	readiness, err := svc.NFCeReadiness(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	// Even if the random number were a valid CNPJ by chance, the explicit
	// invalid check-digit fixture below is guaranteed to be malformed.
	if _, err := pool.Exec(ctx, "UPDATE companies SET cnpj='11222333000180' WHERE id=$1", tenantID); err != nil {
		t.Fatal(err)
	}
	readiness, err = svc.NFCeReadiness(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if readiness.IssuerIdentityConfigured || readiness.ReadyForHomologationData ||
		!slices.Contains(readiness.BlockingReasons, "issuer_cnpj_invalid") {
		t.Fatalf("invalid check digit was not blocked: %+v", readiness)
	}
	if _, err := pool.Exec(ctx, "UPDATE companies SET cnpj='11222333000181' WHERE id=$1", tenantID); err != nil {
		t.Fatal(err)
	}
	readiness, err = svc.NFCeReadiness(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if !readiness.IssuerIdentityConfigured || slices.Contains(readiness.BlockingReasons, "issuer_cnpj_invalid") {
		t.Fatalf("valid structural CNPJ incorrectly blocked: %+v", readiness)
	}
	if _, err := pool.Exec(ctx, "UPDATE companies SET address_city_code='3550308' WHERE id=$1", tenantID); err != nil {
		t.Fatal(err)
	}
	readiness, err = svc.NFCeReadiness(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if readiness.MunicipalityCodeConfigured || readiness.ReadyForHomologationData ||
		!slices.Contains(readiness.BlockingReasons, "issuer_municipality_uf_mismatch") {
		t.Fatalf("non-MG municipality prefix not blocked: %+v", readiness)
	}
	if _, err := pool.Exec(ctx, "UPDATE companies SET address_city_code='3106200', crt='' WHERE id=$1", tenantID); err != nil {
		t.Fatal(err)
	}
	readiness, err = svc.NFCeReadiness(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	if readiness.IssuerIdentityConfigured || !slices.Contains(readiness.BlockingReasons, "issuer_crt_invalid") {
		t.Fatalf("unspecified CRT was guessed or ignored: %+v", readiness)
	}
}
