package sefaz

import (
	"strings"
	"testing"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/example/sistemaemgo/internal/platform"
)

func taxRateForXMLTest(t *testing.T, value string) fisc.TaxRate {
	t.Helper()
	rate, err := fisc.ParseTaxRate(value)
	if err != nil {
		t.Fatal(err)
	}
	return rate
}

func taxXMLFixture(t *testing.T) (fisc.SaleItemFiscalSnapshot, fisc.InvoiceItemTaxCalculation) {
	t.Helper()
	cst := "000"
	classification := "000001"
	snapshot := fisc.SaleItemFiscalSnapshot{
		TenantID:             "tenant",
		SaleItemID:           "item",
		SaleID:               "sale",
		ProductID:            "product",
		NCM:                  "61091000",
		CFOP:                 "5102",
		ICMSOrigin:           "0",
		ICMSRegime:           "csosn",
		ICMSCode:             "102",
		PISCST:               "49",
		COFINSCST:            "49",
		IBSCBSCST:            &cst,
		IBSCBSClassification: &classification,
		ReferenceVersion:     "nfe-010e-v1.02|rtc-2026",
	}
	rtc, err := fisc.CalculateRegularIBSCBS(fisc.RegularIBSCBSInput{
		CST:            cst,
		Classification: classification,
		Base:           platform.NewMoneyCents(100_00),
		IBSUF: fisc.RegularTaxComponentInput{
			Rate:      taxRateForXMLTest(t, "1.0000"),
			Reduction: taxRateForXMLTest(t, "60.0000"),
		},
		IBSMunicipal: fisc.RegularTaxComponentInput{
			Rate: taxRateForXMLTest(t, "0.1000"),
		},
		CBS: fisc.RegularTaxComponentInput{
			Rate: taxRateForXMLTest(t, "0.9000"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	calculation := fisc.InvoiceItemTaxCalculation{
		TenantID:          "tenant",
		InvoiceID:         "invoice",
		SaleID:            "sale",
		SaleItemID:        "item",
		CalculationVersion: "test",
		LegacyTax: fisc.LegacyTaxCalculation{
			ICMS: fisc.LegacyICMSTax{Origin: "0", Regime: "csosn", Code: "102"},
			PIS: fisc.LegacyContributionTax{CST: "49"},
			COFINS: fisc.LegacyContributionTax{CST: "49"},
		},
		RTCTax: fisc.RTCTaxCalculation{IBSCBS: &rtc},
	}
	return snapshot, calculation
}

func TestRenderItemTaxXMLSupportedSimpleNationalRTC(t *testing.T) {
	snapshot, calculation := taxXMLFixture(t)
	content, err := RenderItemTaxXML("1", snapshot, calculation)
	if err != nil {
		t.Fatalf("RenderItemTaxXML: %v", err)
	}
	xml := string(content)
	for _, want := range []string{
		"<imposto>",
		"<ICMS><ICMSSN102><orig>0</orig><CSOSN>102</CSOSN></ICMSSN102></ICMS>",
		"<IBSCBS><CST>000</CST><cClassTrib>000001</cClassTrib><gIBSCBS>",
		"<vBC>100.00</vBC>",
		"<gIBSUF><pIBSUF>1.0000</pIBSUF><gRed><pRedAliq>60.0000</pRedAliq><pAliqEfet>0.4000</pAliqEfet></gRed><vIBSUF>0.40</vIBSUF></gIBSUF>",
		"<gIBSMun><pIBSMun>0.1000</pIBSMun><vIBSMun>0.10</vIBSMun></gIBSMun>",
		"<vIBS>0.50</vIBS>",
		"<gCBS><pCBS>0.9000</pCBS><vCBS>0.90</vCBS></gCBS>",
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("XML missing %q: %s", want, xml)
		}
	}
}

func TestRenderItemTaxXMLMEIRules(t *testing.T) {
	snapshot, calculation := taxXMLFixture(t)
	if _, err := RenderItemTaxXML("4", snapshot, calculation); err != nil {
		t.Fatalf("MEI CSOSN 102 / CFOP 5102 should be supported: %v", err)
	}

	snapshot.ICMSCode = "103"
	calculation.LegacyTax.ICMS.Code = "103"
	if _, err := RenderItemTaxXML("4", snapshot, calculation); err == nil {
		t.Fatal("expected MEI NFC-e CSOSN 103 to be rejected")
	}

	snapshot.ICMSCode = "102"
	calculation.LegacyTax.ICMS.Code = "102"
	snapshot.CFOP = "6102"
	if _, err := RenderItemTaxXML("4", snapshot, calculation); err == nil {
		t.Fatal("expected MEI NFC-e CFOP other than 5102 to be rejected")
	}
}

func TestRenderItemTaxXMLRejectsClassificationMismatch(t *testing.T) {
	snapshot, calculation := taxXMLFixture(t)
	other := "200001"
	snapshot.IBSCBSClassification = &other
	if _, err := RenderItemTaxXML("1", snapshot, calculation); err == nil {
		t.Fatal("expected IBS/CBS classification mismatch")
	}
}

func TestRenderItemTaxXMLRejectsSelectiveTaxUntilRendererExists(t *testing.T) {
	snapshot, calculation := taxXMLFixture(t)
	cst := "000"
	classification := "000001"
	snapshot.ISCST = &cst
	snapshot.ISClassification = &classification
	if _, err := RenderItemTaxXML("1", snapshot, calculation); err == nil {
		t.Fatal("expected selective tax to stay blocked")
	}
}
