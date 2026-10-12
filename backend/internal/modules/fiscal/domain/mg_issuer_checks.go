package domain

import "strings"

// NFCeMGIssuerLocalBlockers returns deterministic *local* checks for the
// configured issuing company. These are not authoritative registrations:
// no SEF/MG credenciamento, IBGE-code existence, state registration, company
// tax regime or certificate validity can be proved from these fields.
// In particular, an informed CRT is NOT an accountant-confirmed CRT.
func NFCeMGIssuerLocalBlockers(p NFCeIssuerProfile) []string {
	reasons := make([]string, 0, 12)
	if strings.TrimSpace(p.LegalName) == "" {
		reasons = append(reasons, "issuer_legal_name_missing")
	}
	if ValidateCNPJ(p.CNPJ) != nil {
		reasons = append(reasons, "issuer_cnpj_invalid")
	}
	if strings.TrimSpace(p.IE) == "" {
		reasons = append(reasons, "issuer_ie_missing")
	}
	if !ValidNFCeCRT(p.CRT) {
		reasons = append(reasons, "issuer_crt_invalid")
	}
	if strings.TrimSpace(p.AddressStreet) == "" {
		reasons = append(reasons, "issuer_street_missing")
	}
	if strings.TrimSpace(p.AddressNumber) == "" {
		reasons = append(reasons, "issuer_number_missing")
	}
	if strings.TrimSpace(p.AddressNeighborhood) == "" {
		reasons = append(reasons, "issuer_neighborhood_missing")
	}
	if strings.TrimSpace(p.AddressCity) == "" {
		reasons = append(reasons, "issuer_city_missing")
	}
	if zip := strings.TrimSpace(p.AddressZIP); len(zip) != 8 || !onlyASCIIDigits(zip) {
		reasons = append(reasons, "issuer_zip_invalid")
	}
	if reason := NFCeMGJurisdictionBlocker(p.AddressState, p.AddressCityCode); reason != "" {
		reasons = append(reasons, reason)
	}
	return reasons
}

func onlyASCIIDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
