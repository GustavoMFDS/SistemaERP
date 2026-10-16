package domain

import "strings"

// NFCeMGJurisdictionBlocker reports a local setup obstacle before a fiscal
// preflight can be interpreted as having all basic inputs present. The current
// NFC-e gateway implements MG only. This is NOT official IBGE validation:
// the municipality still requires verification against an authoritative table.
func NFCeMGJurisdictionBlocker(uf, municipalityCode string) string {
	if strings.ToUpper(strings.TrimSpace(uf)) != "MG" {
		return "issuer_uf_unsupported"
	}
	code := strings.TrimSpace(municipalityCode)
	if len(code) != 7 || !strings.HasPrefix(code, "31") {
		return "issuer_municipality_uf_mismatch"
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return "issuer_municipality_uf_mismatch"
		}
	}
	return ""
}

// NFC-e CRT must come from the issuer's verified tax regime, never from a
// guess based on company size, whether it is "normal", or the prior POS.
// CRT=4 remains a legal code for other tenants; this helper does not
// silently classify a non-MEI tenant as Simples Nacional or Regime Normal.
func ValidNFCeCRT(crt string) bool {
	switch strings.TrimSpace(crt) {
	case "1", "2", "3", "4":
		return true
	default:
		return false
	}
}
