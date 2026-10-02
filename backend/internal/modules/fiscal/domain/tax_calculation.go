package domain

import "github.com/example/sistemaemgo/internal/platform"

type LegacyICMSTax struct {
	Origin string `json:"origin"`
	Regime string `json:"regime"`
	Code   string `json:"code"`
}

type LegacyContributionTax struct {
	CST string `json:"cst"`
}

type LegacyTaxCalculation struct {
	ICMS   LegacyICMSTax         `json:"icms"`
	PIS    LegacyContributionTax `json:"pis"`
	COFINS LegacyContributionTax `json:"cofins"`
}

type SelectiveTaxCalculation struct {
	CST            string         `json:"cst"`
	Classification string         `json:"classification"`
	Base           platform.Money `json:"base"`
	Rate           TaxRate        `json:"rate"`
	Value          platform.Money `json:"value"`
}

type RTCTaxCalculation struct {
	IBSCBS *RegularIBSCBSResult     `json:"ibs_cbs,omitempty"`
	IS     *SelectiveTaxCalculation `json:"is,omitempty"`
}

type InvoiceItemTaxCalculation struct {
	TenantID           string               `json:"tenant_id"`
	InvoiceID          string               `json:"invoice_id"`
	SaleID             string               `json:"sale_id"`
	SaleItemID         string               `json:"sale_item_id"`
	CalculationVersion string               `json:"calculation_version"`
	LegacyTax          LegacyTaxCalculation `json:"legacy_tax"`
	RTCTax             RTCTaxCalculation    `json:"rtc_tax"`
	CalculationSHA256  string               `json:"calculation_sha256"`
}
