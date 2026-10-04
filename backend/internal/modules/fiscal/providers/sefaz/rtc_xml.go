package sefaz

import (
	"fmt"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/example/sistemaemgo/internal/platform"
)

type ibscbsXML struct {
	CST            string       `xml:"CST"`
	Classification string       `xml:"cClassTrib"`
	Group          gIBSCBSXML   `xml:"gIBSCBS"`
}

type gIBSCBSXML struct {
	Base         string         `xml:"vBC"`
	IBSUF        ibsUFItemXML   `xml:"gIBSUF"`
	IBSMunicipal ibsMunItemXML  `xml:"gIBSMun"`
	IBSTotal     string         `xml:"vIBS"`
	CBS          cbsItemXML     `xml:"gCBS"`
}

type ibsUFItemXML struct {
	Rate      string           `xml:"pIBSUF"`
	Reduction *rtcReductionXML `xml:"gRed,omitempty"`
	Value     string           `xml:"vIBSUF"`
}

type ibsMunItemXML struct {
	Rate      string           `xml:"pIBSMun"`
	Reduction *rtcReductionXML `xml:"gRed,omitempty"`
	Value     string           `xml:"vIBSMun"`
}

type cbsItemXML struct {
	Rate      string           `xml:"pCBS"`
	Reduction *rtcReductionXML `xml:"gRed,omitempty"`
	Value     string           `xml:"vCBS"`
}

type rtcReductionXML struct {
	Reduction     string `xml:"pRedAliq"`
	EffectiveRate string `xml:"pAliqEfet"`
}

type ibsCBSTotalXML struct {
	Base string          `xml:"vBCIBSCBS"`
	IBS  ibsTotalXML     `xml:"gIBS"`
	CBS  cbsTotalXML     `xml:"gCBS"`
}

type ibsTotalXML struct {
	UF                ibsUFTotalXML  `xml:"gIBSUF"`
	Municipal         ibsMunTotalXML `xml:"gIBSMun"`
	Value             string         `xml:"vIBS"`
	PresumedCredit    string         `xml:"vCredPres"`
	SuspendedPresumed string         `xml:"vCredPresCondSus"`
}

type ibsUFTotalXML struct {
	Deferred  string `xml:"vDif"`
	Returned  string `xml:"vDevTrib"`
	Value     string `xml:"vIBSUF"`
}

type ibsMunTotalXML struct {
	Deferred string `xml:"vDif"`
	Returned string `xml:"vDevTrib"`
	Value    string `xml:"vIBSMun"`
}

type cbsTotalXML struct {
	PresumedCredit    string `xml:"vCredPres"`
	SuspendedPresumed string `xml:"vCredPresCondSus"`
	Deferred          string `xml:"vDif"`
	Returned          string `xml:"vDevTrib"`
	Value             string `xml:"vCBS"`
}

type rtcTotalsAccumulator struct {
	hasRTC       bool
	base         platform.Money
	ibsUF        platform.Money
	ibsMunicipal platform.Money
	ibs          platform.Money
	cbs          platform.Money
}

func buildIBSCBSXML(
	calculation fisc.InvoiceItemTaxCalculation,
	netValue platform.Money,
) (*ibscbsXML, error) {
	if calculation.RTCTax.IS != nil {
		return nil, fmt.Errorf("selective tax XML is not enabled for the 2026 NFC-e profile")
	}
	rtc := calculation.RTCTax.IBSCBS
	if rtc == nil {
		return nil, nil
	}
	if rtc.Base != netValue {
		return nil, fmt.Errorf(
			"IBS/CBS base %s does not match item net value %s",
			rtc.Base.DBString(),
			netValue.DBString(),
		)
	}
	if rtc.CST == "" || rtc.Classification == "" {
		return nil, fmt.Errorf("IBS/CBS CST and classification are required")
	}
	return &ibscbsXML{
		CST:            rtc.CST,
		Classification: rtc.Classification,
		Group: gIBSCBSXML{
			Base:         rtc.Base.DBString(),
			IBSUF:        renderIBSUFItem(rtc.IBSUF),
			IBSMunicipal: renderIBSMunItem(rtc.IBSMunicipal),
			IBSTotal:     rtc.IBSTotal.DBString(),
			CBS:          renderCBSItem(rtc.CBS),
		},
	}, nil
}

func renderIBSUFItem(input fisc.RegularTaxComponentResult) ibsUFItemXML {
	return ibsUFItemXML{
		Rate:      input.Rate.String(),
		Reduction: renderRTCReduction(input),
		Value:     input.Value.DBString(),
	}
}

func renderIBSMunItem(input fisc.RegularTaxComponentResult) ibsMunItemXML {
	return ibsMunItemXML{
		Rate:      input.Rate.String(),
		Reduction: renderRTCReduction(input),
		Value:     input.Value.DBString(),
	}
}

func renderCBSItem(input fisc.RegularTaxComponentResult) cbsItemXML {
	return cbsItemXML{
		Rate:      input.Rate.String(),
		Reduction: renderRTCReduction(input),
		Value:     input.Value.DBString(),
	}
}

func renderRTCReduction(input fisc.RegularTaxComponentResult) *rtcReductionXML {
	if input.Reduction == 0 {
		return nil
	}
	return &rtcReductionXML{
		Reduction:     input.Reduction.String(),
		EffectiveRate: input.EffectiveRate.String(),
	}
}

func (a *rtcTotalsAccumulator) Add(calculation fisc.InvoiceItemTaxCalculation) error {
	rtc := calculation.RTCTax.IBSCBS
	if rtc == nil {
		return nil
	}
	var err error
	a.hasRTC = true
	a.base, err = a.base.AddChecked(rtc.Base)
	if err != nil {
		return err
	}
	a.ibsUF, err = a.ibsUF.AddChecked(rtc.IBSUF.Value)
	if err != nil {
		return err
	}
	a.ibsMunicipal, err = a.ibsMunicipal.AddChecked(rtc.IBSMunicipal.Value)
	if err != nil {
		return err
	}
	a.ibs, err = a.ibs.AddChecked(rtc.IBSTotal)
	if err != nil {
		return err
	}
	a.cbs, err = a.cbs.AddChecked(rtc.CBS.Value)
	return err
}

func (a rtcTotalsAccumulator) XML() *ibsCBSTotalXML {
	if !a.hasRTC {
		return nil
	}
	return &ibsCBSTotalXML{
		Base: a.base.DBString(),
		IBS: ibsTotalXML{
			UF: ibsUFTotalXML{
				Deferred: "0.00",
				Returned: "0.00",
				Value:    a.ibsUF.DBString(),
			},
			Municipal: ibsMunTotalXML{
				Deferred: "0.00",
				Returned: "0.00",
				Value:    a.ibsMunicipal.DBString(),
			},
			Value:             a.ibs.DBString(),
			PresumedCredit:    "0.00",
			SuspendedPresumed: "0.00",
		},
		CBS: cbsTotalXML{
			PresumedCredit:    "0.00",
			SuspendedPresumed: "0.00",
			Deferred:          "0.00",
			Returned:          "0.00",
			Value:             a.cbs.DBString(),
		},
	}
}
