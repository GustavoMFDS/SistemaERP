package domain

import (
	"fmt"

	"github.com/example/sistemaemgo/internal/platform"
)

type NFCeFiscalTotals struct {
	LegacyDocumentTotal platform.Money `json:"legacy_document_total"`
	IBSBase             platform.Money `json:"ibs_base"`
	IBSUF               platform.Money `json:"ibs_uf"`
	IBSMunicipal        platform.Money `json:"ibs_municipal"`
	IBSTotal            platform.Money `json:"ibs_total"`
	CBS                 platform.Money `json:"cbs"`
	IS                  platform.Money `json:"is"`
	RTCTaxTotal         platform.Money `json:"rtc_tax_total"`
	FiscalTotal         platform.Money `json:"fiscal_total"`
}

func CalculateNFCeFiscalTotals(
	legacyDocumentTotal platform.Money,
	calculations []InvoiceItemTaxCalculation,
) (NFCeFiscalTotals, error) {
	if legacyDocumentTotal < 0 {
		return NFCeFiscalTotals{}, fmt.Errorf("legacy document total must not be negative")
	}
	if len(calculations) == 0 {
		return NFCeFiscalTotals{}, fmt.Errorf("at least one item tax calculation is required")
	}

	out := NFCeFiscalTotals{LegacyDocumentTotal: legacyDocumentTotal}
	var err error
	for _, calculation := range calculations {
		if calculation.RTCTax.IBSCBS != nil {
			rtc := calculation.RTCTax.IBSCBS
			out.IBSBase, err = out.IBSBase.AddChecked(rtc.Base)
			if err != nil {
				return NFCeFiscalTotals{}, err
			}
			out.IBSUF, err = out.IBSUF.AddChecked(rtc.IBSUF.Value)
			if err != nil {
				return NFCeFiscalTotals{}, err
			}
			out.IBSMunicipal, err = out.IBSMunicipal.AddChecked(rtc.IBSMunicipal.Value)
			if err != nil {
				return NFCeFiscalTotals{}, err
			}
			out.IBSTotal, err = out.IBSTotal.AddChecked(rtc.IBSTotal)
			if err != nil {
				return NFCeFiscalTotals{}, err
			}
			out.CBS, err = out.CBS.AddChecked(rtc.CBS.Value)
			if err != nil {
				return NFCeFiscalTotals{}, err
			}
		}
		if calculation.RTCTax.IS != nil {
			out.IS, err = out.IS.AddChecked(calculation.RTCTax.IS.Value)
			if err != nil {
				return NFCeFiscalTotals{}, err
			}
		}
	}

	ibsAndCBS, err := out.IBSTotal.AddChecked(out.CBS)
	if err != nil {
		return NFCeFiscalTotals{}, err
	}
	out.RTCTaxTotal, err = ibsAndCBS.AddChecked(out.IS)
	if err != nil {
		return NFCeFiscalTotals{}, err
	}
	out.FiscalTotal, err = out.LegacyDocumentTotal.AddChecked(out.RTCTaxTotal)
	if err != nil {
		return NFCeFiscalTotals{}, err
	}
	return out, nil
}

func ValidateNFCePaidTotal(paid, fiscalTotal platform.Money) error {
	if paid < 0 || fiscalTotal < 0 {
		return fmt.Errorf("payment and fiscal totals must not be negative")
	}
	if paid != fiscalTotal {
		return fmt.Errorf("payment total %s does not match fiscal total %s", paid.DBString(), fiscalTotal.DBString())
	}
	return nil
}
