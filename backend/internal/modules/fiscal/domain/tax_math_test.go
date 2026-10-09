package domain

import (
	"encoding/json"
	"testing"

	"github.com/example/sistemaemgo/internal/platform"
)

func mustTaxRate(t *testing.T, raw string) TaxRate {
	t.Helper()
	rate, err := ParseTaxRate(raw)
	if err != nil {
		t.Fatalf("ParseTaxRate(%q): %v", raw, err)
	}
	return rate
}

func TestTaxRateParseFormatAndJSON(t *testing.T) {
	rate := mustTaxRate(t, "0.1")
	if got := rate.String(); got != "0.1000" {
		t.Fatalf("String=%s, want 0.1000", got)
	}
	content, err := json.Marshal(rate)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "\"0.1000\"" {
		t.Fatalf("JSON=%s", content)
	}
	var decoded TaxRate
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != rate {
		t.Fatalf("decoded=%d, want %d", decoded, rate)
	}
}

func TestTaxRateRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{"", "-1", "+1", "1.00001", "100.0001", "abc"} {
		if _, err := ParseTaxRate(raw); err == nil {
			t.Fatalf("ParseTaxRate(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestApplyTaxRateUsesCentRoundingWithoutFloat(t *testing.T) {
	base := platform.NewMoneyCents(10_00)
	value, err := ApplyTaxRate(base, mustTaxRate(t, "0.1000"))
	if err != nil {
		t.Fatal(err)
	}
	if value.Cents() != 1 {
		t.Fatalf("value=%s, want 0.01", value.DBString())
	}

	base = platform.NewMoneyCents(50)
	value, err = ApplyTaxRate(base, mustTaxRate(t, "1.0000"))
	if err != nil {
		t.Fatal(err)
	}
	if value.Cents() != 1 {
		t.Fatalf("half-cent rounding=%s, want 0.01", value.DBString())
	}
}

func TestEffectiveTaxRate(t *testing.T) {
	effective, err := EffectiveTaxRate(
		mustTaxRate(t, "1.0000"),
		mustTaxRate(t, "60.0000"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := effective.String(); got != "0.4000" {
		t.Fatalf("effective=%s, want 0.4000", got)
	}
}

func TestCalculateRegularIBSCBS(t *testing.T) {
	result, err := CalculateRegularIBSCBS(RegularIBSCBSInput{
		CST:            "000",
		Classification: "000001",
		Base:           platform.NewMoneyCents(100_00),
		IBSUF: RegularTaxComponentInput{
			Rate: mustTaxRate(t, "0.1000"),
		},
		IBSMunicipal: RegularTaxComponentInput{
			Rate: mustTaxRate(t, "0.0000"),
		},
		CBS: RegularTaxComponentInput{
			Rate: mustTaxRate(t, "0.9000"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IBSUF.Value.DBString() != "0.10" {
		t.Fatalf("IBS UF=%s", result.IBSUF.Value.DBString())
	}
	if result.IBSTotal.DBString() != "0.10" {
		t.Fatalf("IBS total=%s", result.IBSTotal.DBString())
	}
	if result.CBS.Value.DBString() != "0.90" {
		t.Fatalf("CBS=%s", result.CBS.Value.DBString())
	}
}

func TestCalculateRegularIBSCBSRejectsClassificationMismatch(t *testing.T) {
	_, err := CalculateRegularIBSCBS(RegularIBSCBSInput{
		CST:            "000",
		Classification: "200001",
		Base:           platform.NewMoneyCents(100),
	})
	if err == nil {
		t.Fatal("expected cClassTrib/CST mismatch")
	}
}

func TestValidateNFCeReferenceRates2026(t *testing.T) {
	result, err := CalculateRegularIBSCBS(RegularIBSCBSInput{
		CST:            "000",
		Classification: "000001",
		Base:           platform.NewMoneyCents(100_00),
		IBSUF: RegularTaxComponentInput{
			Rate: mustTaxRate(t, "0.1000"),
		},
		IBSMunicipal: RegularTaxComponentInput{
			Rate: mustTaxRate(t, "0.0000"),
		},
		CBS: RegularTaxComponentInput{
			Rate: mustTaxRate(t, "0.9000"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateNFCeReferenceRates2026(result); err != nil {
		t.Fatalf("official 2026 reference rates rejected: %v", err)
	}

	result.CBS.Rate = mustTaxRate(t, "1.0000")
	if err := ValidateNFCeReferenceRates2026(result); err == nil {
		t.Fatal("expected non-reference 2026 CBS rate to be rejected")
	}
}
