package domain

import (
	"testing"

	"github.com/example/sistemaemgo/internal/platform"
)

func fiscalTotalsRate(t *testing.T, raw string) TaxRate {
	t.Helper()
	value, err := ParseTaxRate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func fiscalTotalsRTC(t *testing.T, baseCents int64) *RegularIBSCBSResult {
	t.Helper()
	result, err := CalculateRegularIBSCBS(RegularIBSCBSInput{
		CST:            "000",
		Classification: "000001",
		Base:           platform.NewMoneyCents(baseCents),
		IBSUF: RegularTaxComponentInput{
			Rate: fiscalTotalsRate(t, "1.0000"),
		},
		IBSMunicipal: RegularTaxComponentInput{
			Rate: fiscalTotalsRate(t, "0.1000"),
		},
		CBS: RegularTaxComponentInput{
			Rate: fiscalTotalsRate(t, "0.9000"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &result
}

func TestCalculateNFCeFiscalTotalsSumsRTCByItem(t *testing.T) {
	calculations := []InvoiceItemTaxCalculation{
		{RTCTax: RTCTaxCalculation{IBSCBS: fiscalTotalsRTC(t, 100_00)}},
		{RTCTax: RTCTaxCalculation{IBSCBS: fiscalTotalsRTC(t, 50_00)}},
	}

	got, err := CalculateNFCeFiscalTotals(platform.NewMoneyCents(150_00), calculations)
	if err != nil {
		t.Fatal(err)
	}
	if got.IBSBase.DBString() != "150.00" {
		t.Fatalf("IBS base=%s, want 150.00", got.IBSBase.DBString())
	}
	if got.IBSUF.DBString() != "1.50" {
		t.Fatalf("IBS UF=%s, want 1.50", got.IBSUF.DBString())
	}
	if got.IBSMunicipal.DBString() != "0.15" {
		t.Fatalf("IBS municipal=%s, want 0.15", got.IBSMunicipal.DBString())
	}
	if got.IBSTotal.DBString() != "1.65" {
		t.Fatalf("IBS total=%s, want 1.65", got.IBSTotal.DBString())
	}
	if got.CBS.DBString() != "1.35" {
		t.Fatalf("CBS=%s, want 1.35", got.CBS.DBString())
	}
	if got.RTCTaxTotal.DBString() != "3.00" {
		t.Fatalf("RTC total=%s, want 3.00", got.RTCTaxTotal.DBString())
	}
	if got.FiscalTotal.DBString() != "153.00" {
		t.Fatalf("fiscal total=%s, want 153.00", got.FiscalTotal.DBString())
	}
}

func TestCalculateNFCeFiscalTotalsIncludesSelectiveTax(t *testing.T) {
	calculation := InvoiceItemTaxCalculation{
		RTCTax: RTCTaxCalculation{
			IS: &SelectiveTaxCalculation{
				CST:            "000",
				Classification: "000001",
				Base:           platform.NewMoneyCents(100_00),
				Rate:           fiscalTotalsRate(t, "2.0000"),
				Value:          platform.NewMoneyCents(2_00),
			},
		},
	}
	got, err := CalculateNFCeFiscalTotals(
		platform.NewMoneyCents(100_00),
		[]InvoiceItemTaxCalculation{calculation},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.IS.DBString() != "2.00" || got.FiscalTotal.DBString() != "102.00" {
		t.Fatalf("unexpected totals: %+v", got)
	}
}

func TestCalculateNFCeFiscalTotalsLegacyOnly(t *testing.T) {
	got, err := CalculateNFCeFiscalTotals(
		platform.NewMoneyCents(10_00),
		[]InvoiceItemTaxCalculation{{}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.RTCTaxTotal != 0 || got.FiscalTotal.DBString() != "10.00" {
		t.Fatalf("unexpected legacy-only totals: %+v", got)
	}
}

func TestCalculateNFCeFiscalTotalsRejectsInvalidInputs(t *testing.T) {
	if _, err := CalculateNFCeFiscalTotals(platform.NewMoneyCents(-1), []InvoiceItemTaxCalculation{{}}); err == nil {
		t.Fatal("expected negative legacy total to fail")
	}
	if _, err := CalculateNFCeFiscalTotals(platform.NewMoneyCents(100), nil); err == nil {
		t.Fatal("expected empty calculations to fail")
	}
}

func TestValidateNFCePaidTotal(t *testing.T) {
	if err := ValidateNFCePaidTotal(
		platform.NewMoneyCents(153_00),
		platform.NewMoneyCents(153_00),
	); err != nil {
		t.Fatalf("matching payment rejected: %v", err)
	}
	if err := ValidateNFCePaidTotal(
		platform.NewMoneyCents(150_00),
		platform.NewMoneyCents(153_00),
	); err == nil {
		t.Fatal("expected mismatched payment total to fail")
	}
}
