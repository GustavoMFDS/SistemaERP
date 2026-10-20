package domain

import "testing"

func TestNFCeMGJurisdictionBlocker(t *testing.T) {
	tests := []struct {
		name, uf, municipality, reason string
	}{
		{"valid MG IBGE shape", "MG", "3106200", ""},
		{"case and whitespace", " mg ", " 3106200 ", ""},
		{"unsupported state", "SP", "3550308", "issuer_uf_unsupported"},
		{"missing state", "", "3106200", "issuer_uf_unsupported"},
		{"other-state code", "MG", "3550308", "issuer_municipality_uf_mismatch"},
		{"short code", "MG", "310620", "issuer_municipality_uf_mismatch"},
		{"nondigit code", "MG", "31A6200", "issuer_municipality_uf_mismatch"},
		{"missing code", "MG", "", "issuer_municipality_uf_mismatch"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NFCeMGJurisdictionBlocker(tt.uf, tt.municipality)
			if got != tt.reason {
				t.Fatalf("blocker=%q, want %q", got, tt.reason)
			}
		})
	}
}

func TestNFCeCRTRequiresExplicitRecognizedValue(t *testing.T) {
	for _, valid := range []string{"1", "2", "3", "4", " 3 "} {
		if !ValidNFCeCRT(valid) {
			t.Errorf("CRT %q should be accepted", valid)
		}
	}
	for _, invalid := range []string{"", " ", "0", "5", "MEI", "normal", "simples"} {
		if ValidNFCeCRT(invalid) {
			t.Errorf("CRT %q must not be guessed or accepted", invalid)
		}
	}
}
