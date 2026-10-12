package application

import (
	"testing"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

func TestProductionNFCeDataReadyFailClosedOnDetailedIssuerBlockers(t *testing.T) {
	// Isolates the data predicate; real transmission additionally requires a
	// valid signing/SEFAZ stack, credentials and explicit official homologation.
	good := fisc.NFCeReadiness{
		ConfigExists: true,
		Environment: "production",
		IssuerIdentityConfigured: true,
		IssuerAddressConfigured: true,
		MunicipalityCodeConfigured: true,
		CertificateReferenceConfigured: true,
		ActiveProducts: 3,
		ProductsMissingNCM: 0,
		ProductsMissingFiscalProfile: 0,
		BlockingReasons: []string{"homologation_environment"},
	}
	if !productionNFCeDataReady(good) {
		t.Fatal("baseline locally filled fields rejected")
	}
	for _, blocker := range []string{
		"issuer_cnpj_invalid", "issuer_crt_invalid",
		"issuer_zip_invalid", "issuer_municipality_uf_mismatch",
		"issuer_uf_unsupported", "issuer_ie_missing",
		"product_ncm", "product_fiscal_profile",
	} {
		t.Run(blocker, func(t *testing.T) {
			p := good
			p.BlockingReasons = append([]string(nil), good.BlockingReasons...)
			p.BlockingReasons = append(p.BlockingReasons, blocker)
			if productionNFCeDataReady(p) {
				t.Fatal("fiscal activation must reject detailed local blockers")
			}
		})
	}
	p := good
	p.IssuerIdentityConfigured = false
	if productionNFCeDataReady(p) {
		t.Fatal("identity flag may not be bypassed")
	}
}
