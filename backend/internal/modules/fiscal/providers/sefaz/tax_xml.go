package sefaz

import (
	"encoding/xml"
	"fmt"
	"strings"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

type itemTaxXML struct {
	XMLName xml.Name   `xml:"imposto"`
	ICMS    itemICMSXML `xml:"ICMS"`
	IBSCBS  *itemIBSCBSXML `xml:"IBSCBS,omitempty"`
}

type itemICMSXML struct {
	SN102 *itemICMSSN102XML `xml:"ICMSSN102,omitempty"`
}

type itemICMSSN102XML struct {
	Origin string `xml:"orig"`
	CSOSN  string `xml:"CSOSN"`
}

type itemIBSCBSXML struct {
	CST            string          `xml:"CST"`
	Classification string          `xml:"cClassTrib"`
	Group          itemGIBSCBSXML  `xml:"gIBSCBS"`
}

type itemGIBSCBSXML struct {
	Base         string                 `xml:"vBC"`
	IBSUF        itemIBSComponentXML    `xml:"gIBSUF"`
	IBSMunicipal itemIBSComponentXML    `xml:"gIBSMun"`
	IBSTotal     string                 `xml:"vIBS"`
	CBS          itemCBSComponentXML    `xml:"gCBS"`
}

type itemIBSComponentXML struct {
	Rate      string              `xml:"pIBSUF,omitempty"`
	MunRate   string              `xml:"pIBSMun,omitempty"`
	Reduction *itemReductionXML   `xml:"gRed,omitempty"`
	Value     string              `xml:"vIBSUF,omitempty"`
	MunValue  string              `xml:"vIBSMun,omitempty"`
}

type itemCBSComponentXML struct {
	Rate      string            `xml:"pCBS"`
	Reduction *itemReductionXML `xml:"gRed,omitempty"`
	Value     string            `xml:"vCBS"`
}

type itemReductionXML struct {
	Reduction     string `xml:"pRedAliq"`
	EffectiveRate string `xml:"pAliqEfet"`
}

func RenderItemTaxXML(
	issuerCRT string,
	snapshot fisc.SaleItemFiscalSnapshot,
	calculation fisc.InvoiceItemTaxCalculation,
) ([]byte, error) {
	if err := validateInitialLegacyTaxProfile(issuerCRT, snapshot, calculation); err != nil {
		return nil, err
	}
	if err := validateCalculationForTaxXML(snapshot, calculation); err != nil {
		return nil, err
	}

	doc := itemTaxXML{
		ICMS: itemICMSXML{
			SN102: &itemICMSSN102XML{
				Origin: snapshot.ICMSOrigin,
				CSOSN:  snapshot.ICMSCode,
			},
		},
	}
	if calculation.RTCTax.IBSCBS != nil {
		rtc := calculation.RTCTax.IBSCBS
		doc.IBSCBS = &itemIBSCBSXML{
			CST:            rtc.CST,
			Classification: rtc.Classification,
			Group: itemGIBSCBSXML{
				Base:         rtc.Base.DBString(),
				IBSUF:        renderIBSUFComponent(rtc.IBSUF),
				IBSMunicipal: renderIBSMunicipalComponent(rtc.IBSMunicipal),
				IBSTotal:     rtc.IBSTotal.DBString(),
				CBS:          renderCBSComponent(rtc.CBS),
			},
		}
	}
	return xml.Marshal(doc)
}

func validateInitialLegacyTaxProfile(
	issuerCRT string,
	snapshot fisc.SaleItemFiscalSnapshot,
	calculation fisc.InvoiceItemTaxCalculation,
) error {
	issuerCRT = strings.TrimSpace(issuerCRT)
	if snapshot.ICMSRegime != "csosn" {
		return fmt.Errorf("unsupported ICMS regime %q for initial NFC-e renderer", snapshot.ICMSRegime)
	}
	switch issuerCRT {
	case "1":
		switch snapshot.ICMSCode {
		case "102", "103", "300", "400":
		default:
			return fmt.Errorf("unsupported CSOSN %s for CRT 1 initial NFC-e renderer", snapshot.ICMSCode)
		}
	case "4":
		switch snapshot.ICMSCode {
		case "102", "300":
		default:
			return fmt.Errorf("unsupported CSOSN %s for CRT 4 NFC-e", snapshot.ICMSCode)
		}
		if snapshot.CFOP != "5102" {
			return fmt.Errorf("CRT 4 NFC-e initial renderer requires CFOP 5102")
		}
	default:
		return fmt.Errorf("unsupported CRT %q for initial NFC-e renderer", issuerCRT)
	}
	if calculation.LegacyTax.ICMS.Origin != snapshot.ICMSOrigin ||
		calculation.LegacyTax.ICMS.Regime != snapshot.ICMSRegime ||
		calculation.LegacyTax.ICMS.Code != snapshot.ICMSCode {
		return fmt.Errorf("legacy ICMS calculation does not match fiscal snapshot")
	}
	return nil
}

func validateCalculationForTaxXML(
	snapshot fisc.SaleItemFiscalSnapshot,
	calculation fisc.InvoiceItemTaxCalculation,
) error {
	if calculation.SaleID != snapshot.SaleID ||
		calculation.SaleItemID != snapshot.SaleItemID {
		return fmt.Errorf("tax calculation does not belong to fiscal snapshot")
	}
	if snapshot.IBSCBSCST == nil && snapshot.IBSCBSClassification == nil {
		if calculation.RTCTax.IBSCBS != nil {
			return fmt.Errorf("unexpected IBS/CBS calculation for snapshot without IBS/CBS classification")
		}
		return nil
	}
	if snapshot.IBSCBSCST == nil || snapshot.IBSCBSClassification == nil ||
		calculation.RTCTax.IBSCBS == nil {
		return fmt.Errorf("missing IBS/CBS calculation")
	}
	if calculation.RTCTax.IBSCBS.CST != *snapshot.IBSCBSCST ||
		calculation.RTCTax.IBSCBS.Classification != *snapshot.IBSCBSClassification {
		return fmt.Errorf("IBS/CBS calculation classification does not match snapshot")
	}
	if snapshot.ISCST != nil || snapshot.ISClassification != nil ||
		calculation.RTCTax.IS != nil {
		return fmt.Errorf("selective tax XML renderer is not implemented yet")
	}
	return nil
}

func renderIBSUFComponent(input fisc.RegularTaxComponentResult) itemIBSComponentXML {
	return itemIBSComponentXML{
		Rate:      input.Rate.String(),
		Reduction: renderReduction(input),
		Value:     input.Value.DBString(),
	}
}

func renderIBSMunicipalComponent(input fisc.RegularTaxComponentResult) itemIBSComponentXML {
	return itemIBSComponentXML{
		MunRate:   input.Rate.String(),
		Reduction: renderReduction(input),
		MunValue:  input.Value.DBString(),
	}
}

func renderCBSComponent(input fisc.RegularTaxComponentResult) itemCBSComponentXML {
	return itemCBSComponentXML{
		Rate:      input.Rate.String(),
		Reduction: renderReduction(input),
		Value:     input.Value.DBString(),
	}
}

func renderReduction(input fisc.RegularTaxComponentResult) *itemReductionXML {
	if input.Reduction == 0 {
		return nil
	}
	return &itemReductionXML{
		Reduction:     input.Reduction.String(),
		EffectiveRate: input.EffectiveRate.String(),
	}
}
