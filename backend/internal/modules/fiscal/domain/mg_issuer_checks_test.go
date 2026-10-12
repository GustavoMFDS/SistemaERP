package domain

import (
	"slices"
	"testing"
)

func validMGIssuerForLocalCheck() NFCeIssuerProfile {
	return NFCeIssuerProfile{
		LegalName: "Mercado Exemplo Ltda",
		CNPJ: "11222333000181",
		IE: "0012345678901",
		CRT: "1",
		AddressStreet: "Rua Exemplo",
		AddressNumber: "120",
		AddressNeighborhood: "Centro",
		AddressCity: "Belo Horizonte",
		AddressCityCode: "3106200",
		AddressState: "MG",
		AddressZIP: "30130010",
	}
}

func TestNFCeMGIssuerLocalChecksDoNotGuessRegime(t *testing.T) {
	valid := validMGIssuerForLocalCheck()
	if got := NFCeMGIssuerLocalBlockers(valid); len(got) != 0 {
		t.Fatalf("valid structural MG issuer should have no local blockers: %v", got)
	}
	// The "not MEI" statement is insufficient to infer CRT=1 or CRT=3.
	noCRT := valid
	noCRT.CRT = ""
	if got := NFCeMGIssuerLocalBlockers(noCRT); !slices.Contains(got, "issuer_crt_invalid") {
		t.Fatalf("unknown CRT must stay pending: %v", got)
	}
	// A formally present CNPJ may still have an invalid check digit.
	badCNPJ := valid
	badCNPJ.CNPJ = "11222333000180"
	if got := NFCeMGIssuerLocalBlockers(badCNPJ); !slices.Contains(got, "issuer_cnpj_invalid") {
		t.Fatalf("invalid CNPJ check digit must be reported: %v", got)
	}
}

func TestNFCeMGIssuerLocalChecksIdentifyPreciseMissingFields(t *testing.T) {
	invalid := validMGIssuerForLocalCheck()
	invalid.IE = " "
	invalid.AddressStreet = ""
	invalid.AddressNumber = ""
	invalid.AddressNeighborhood = ""
	invalid.AddressCity = ""
	invalid.AddressZIP = "30130-010"
	invalid.AddressCityCode = "3550308"
	got := NFCeMGIssuerLocalBlockers(invalid)
	for _, expected := range []string{
		"issuer_ie_missing", "issuer_street_missing", "issuer_number_missing",
		"issuer_neighborhood_missing", "issuer_city_missing",
		"issuer_zip_invalid", "issuer_municipality_uf_mismatch",
	} {
		if !slices.Contains(got, expected) {
			t.Errorf("missing %q in %v", expected, got)
		}
	}
	if slices.Contains(got, "issuer_uf_unsupported") {
		t.Fatal("valid MG UF should not be rejected for incorrect municipality prefix")
	}
}

func TestNFCeMGIssuerLocalChecksDoNotCertifyGovernmentRegisters(t *testing.T) {
	p := validMGIssuerForLocalCheck()
	// The 31 prefix is only a structural check; not a claim that IBGE
	// recognizes this municipality, nor that IE/CRT is registered.
	p.AddressCityCode = "3199999"
	p.IE = "ISENTO"
	if got := NFCeMGIssuerLocalBlockers(p); len(got) != 0 {
		t.Fatalf("structural checks must not masquerade as official IBGE/IE lookup: %v", got)
	}
	for _, zip := range []string{"", "3013001", "301300100", "3013AB10"} {
		p.AddressZIP = zip
		if got := NFCeMGIssuerLocalBlockers(p); !slices.Contains(got, "issuer_zip_invalid") {
			t.Errorf("ZIP %q should be blocked locally: %v", zip, got)
		}
	}
}
