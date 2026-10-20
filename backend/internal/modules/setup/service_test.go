package setup

import "testing"

func TestValidStepSeparatesManualFiscalPreparationFromOfficialApproval(t *testing.T) {
	for _, step := range []string{
		"stock", "team",
		"fiscal_docs_collected",
		"fiscal_siare_access_arranged",
		"fiscal_accountant_consulted",
		"fiscal_previous_series_checked",
	} {
		if !ValidStep(step) {
			t.Errorf("manual checklist step %q should be allowed", step)
		}
	}
	for _, step := range []string{
		"", "fiscal_authorized", "fiscal_homologated",
		"fiscal_sefaz_approved", "fiscal_transmission_enabled",
		"invoice:generate", "fiscal_docs_collected;DROP TABLE companies",
	} {
		if ValidStep(step) {
			t.Errorf("untrusted official/unknown step %q should be rejected", step)
		}
	}
}
