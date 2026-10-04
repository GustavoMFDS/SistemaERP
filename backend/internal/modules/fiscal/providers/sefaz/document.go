package sefaz

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/example/sistemaemgo/internal/platform"
)

const homologationProductDescription = "NOTA FISCAL EMITIDA EM AMBIENTE DE HOMOLOGACAO - SEM VALOR FISCAL"

type UnsignedNFCeItem struct {
	Number        int
	Code          string
	Description   string
	Unit          string
	NCM           string
	CEST          *string
	CFOP          string
	Quantity      platform.Quantity
	UnitPrice     platform.Money
	GrossValue    platform.Money
	DiscountValue platform.Money
	Tax           fisc.InvoiceItemTaxCalculation
}

type UnsignedNFCePayment struct {
	Method string
	Amount platform.Money
}

type UnsignedNFCeInput struct {
	Reservation     fisc.NFCeReservation
	Issuer          fisc.NFCeIssuerProfile
	Items           []UnsignedNFCeItem
	Payments        []UnsignedNFCePayment
	ProcessVersion  string
	QRCodeBaseURL   string
	ConsultationURL string
}

type nfeDocumentXML struct {
	XMLName    xml.Name      `xml:"NFe"`
	Xmlns      string        `xml:"xmlns,attr"`
	InfNFe     infNFeXML     `xml:"infNFe"`
	InfNFeSupl infNFeSuplXML `xml:"infNFeSupl"`
}

type infNFeXML struct {
	ID      string          `xml:"Id,attr"`
	Version string          `xml:"versao,attr"`
	Ide     ideXML          `xml:"ide"`
	Emit    emitXML         `xml:"emit"`
	Det     []detXML        `xml:"det"`
	Total   totalXML        `xml:"total"`
	Transp  transportXML    `xml:"transp"`
	Pag     paymentGroupXML `xml:"pag"`
}

type ideXML struct {
	CUF      string `xml:"cUF"`
	CNF      string `xml:"cNF"`
	NatOp    string `xml:"natOp"`
	Mod      string `xml:"mod"`
	Serie    int    `xml:"serie"`
	NNF      int64  `xml:"nNF"`
	DhEmi    string `xml:"dhEmi"`
	TpNF     int    `xml:"tpNF"`
	IdDest   int    `xml:"idDest"`
	CMunFG   string `xml:"cMunFG"`
	TpImp    int    `xml:"tpImp"`
	TpEmis   int    `xml:"tpEmis"`
	CDV      int    `xml:"cDV"`
	TpAmb    string `xml:"tpAmb"`
	FinNFe   int    `xml:"finNFe"`
	IndFinal int    `xml:"indFinal"`
	IndPres  int    `xml:"indPres"`
	ProcEmi  int    `xml:"procEmi"`
	VerProc  string `xml:"verProc"`
}

type emitXML struct {
	CNPJ      string         `xml:"CNPJ"`
	XNome     string         `xml:"xNome"`
	XFant     *string        `xml:"xFant,omitempty"`
	EnderEmit emitAddressXML `xml:"enderEmit"`
	IE        string         `xml:"IE"`
	CRT       string         `xml:"CRT"`
}

type emitAddressXML struct {
	XLgr    string  `xml:"xLgr"`
	Nro     string  `xml:"nro"`
	XCpl    *string `xml:"xCpl,omitempty"`
	XBairro string  `xml:"xBairro"`
	CMun    string  `xml:"cMun"`
	XMun    string  `xml:"xMun"`
	UF      string  `xml:"UF"`
	CEP     string  `xml:"CEP"`
	CPais   string  `xml:"cPais"`
	XPais   string  `xml:"xPais"`
}

type detXML struct {
	NItem   int        `xml:"nItem,attr"`
	Prod    productXML `xml:"prod"`
	Imposto taxXML     `xml:"imposto"`
}

type productXML struct {
	CProd    string  `xml:"cProd"`
	CEAN     string  `xml:"cEAN"`
	XProd    string  `xml:"xProd"`
	NCM      string  `xml:"NCM"`
	CEST     *string `xml:"CEST,omitempty"`
	CFOP     string  `xml:"CFOP"`
	UCom     string  `xml:"uCom"`
	QCom     string  `xml:"qCom"`
	VUnCom   string  `xml:"vUnCom"`
	VProd    string  `xml:"vProd"`
	CEANTrib string  `xml:"cEANTrib"`
	UTrib    string  `xml:"uTrib"`
	QTrib    string  `xml:"qTrib"`
	VUnTrib  string  `xml:"vUnTrib"`
	VDesc    *string `xml:"vDesc,omitempty"`
	IndTot   int     `xml:"indTot"`
}

type taxXML struct {
	ICMS   icmsXML    `xml:"ICMS"`
	PIS    pisXML     `xml:"PIS"`
	COFINS cofinsXML  `xml:"COFINS"`
	IBSCBS *ibscbsXML `xml:"IBSCBS,omitempty"`
}

type icmsXML struct {
	ICMSSN102 *icmsSN102XML `xml:"ICMSSN102,omitempty"`
	ICMS40    *icms40XML    `xml:"ICMS40,omitempty"`
}

type icmsSN102XML struct {
	Orig  string `xml:"orig"`
	CSOSN string `xml:"CSOSN"`
}

type icms40XML struct {
	Orig string `xml:"orig"`
	CST  string `xml:"CST"`
}

type pisXML struct {
	PISNT *contributionNTXML `xml:"PISNT,omitempty"`
}

type cofinsXML struct {
	COFINSNT *contributionNTXML `xml:"COFINSNT,omitempty"`
}

type contributionNTXML struct {
	CST string `xml:"CST"`
}

type totalXML struct {
	ICMSTot   icmsTotalXML    `xml:"ICMSTot"`
	IBSCBSTot *ibsCBSTotalXML `xml:"IBSCBSTot,omitempty"`
}

type icmsTotalXML struct {
	VBC        string `xml:"vBC"`
	VICMS      string `xml:"vICMS"`
	VICMSDeson string `xml:"vICMSDeson"`
	VFCP       string `xml:"vFCP"`
	VBCST      string `xml:"vBCST"`
	VST        string `xml:"vST"`
	VFCPST     string `xml:"vFCPST"`
	VFCPSTRet  string `xml:"vFCPSTRet"`
	VProd      string `xml:"vProd"`
	VFrete     string `xml:"vFrete"`
	VSeg       string `xml:"vSeg"`
	VDesc      string `xml:"vDesc"`
	VII        string `xml:"vII"`
	VIPI       string `xml:"vIPI"`
	VIPIDevol  string `xml:"vIPIDevol"`
	VPIS       string `xml:"vPIS"`
	VCOFINS    string `xml:"vCOFINS"`
	VOutro     string `xml:"vOutro"`
	VNF        string `xml:"vNF"`
}

type transportXML struct {
	ModFrete int `xml:"modFrete"`
}

type paymentGroupXML struct {
	Details []paymentDetailXML `xml:"detPag"`
}

type paymentDetailXML struct {
	TPag string `xml:"tPag"`
	VPag string `xml:"vPag"`
}

type infNFeSuplXML struct {
	QRCode   string `xml:"qrCode"`
	URLChave string `xml:"urlChave"`
}

func BuildUnsignedNFCeLegacyCandidate(input UnsignedNFCeInput) ([]byte, error) {
	if err := validateUnsignedNFCeInput(input); err != nil {
		return nil, err
	}

	cUF, _ := fisc.UFCode(input.Issuer.AddressState)
	ambient, _ := tpAmb(Environment(input.Reservation.Environment))
	qrCode, err := BuildOnlineQRCodeV3URL(
		input.QRCodeBaseURL,
		Environment(input.Reservation.Environment),
		input.Reservation.AccessKey,
	)
	if err != nil {
		return nil, err
	}
	consultationURL, err := validatePublicFiscalURL(input.ConsultationURL)
	if err != nil {
		return nil, fmt.Errorf("consultation URL: %w", err)
	}

	items := make([]detXML, 0, len(input.Items))
	var productTotal, discountTotal, netTotal platform.Money
	var rtcTotals rtcTotalsAccumulator
	for index, item := range input.Items {
		gross, err := item.UnitPrice.MulQtyChecked(item.Quantity)
		if err != nil || gross != item.GrossValue {
			return nil, fmt.Errorf("item %d gross value mismatch", item.Number)
		}
		net, err := item.GrossValue.SubChecked(item.DiscountValue)
		if err != nil || net < 0 {
			return nil, fmt.Errorf("item %d discount exceeds gross value", item.Number)
		}
		productTotal, err = productTotal.AddChecked(item.GrossValue)
		if err != nil {
			return nil, err
		}
		discountTotal, err = discountTotal.AddChecked(item.DiscountValue)
		if err != nil {
			return nil, err
		}
		netTotal, err = netTotal.AddChecked(net)
		if err != nil {
			return nil, err
		}

		tax, err := buildLegacyTaxXML(item.Tax)
		if err != nil {
			return nil, fmt.Errorf("item %d tax: %w", item.Number, err)
		}
		rtcTax, err := buildIBSCBSXML(item.Tax, net)
		if err != nil {
			return nil, fmt.Errorf("item %d RTC tax: %w", item.Number, err)
		}
		tax.IBSCBS = rtcTax
		if err := rtcTotals.Add(item.Tax); err != nil {
			return nil, fmt.Errorf("item %d RTC total: %w", item.Number, err)
		}
		description := strings.TrimSpace(item.Description)
		if index == 0 && input.Reservation.Environment == string(EnvironmentHomologation) {
			description = homologationProductDescription
		}
		var discount *string
		if item.DiscountValue > 0 {
			value := item.DiscountValue.DBString()
			discount = &value
		}
		items = append(items, detXML{
			NItem: item.Number,
			Prod: productXML{
				CProd:    strings.TrimSpace(item.Code),
				CEAN:     "SEM GTIN",
				XProd:    description,
				NCM:      strings.TrimSpace(item.NCM),
				CEST:     normalizedOptional(item.CEST),
				CFOP:     strings.TrimSpace(item.CFOP),
				UCom:     strings.ToUpper(strings.TrimSpace(item.Unit)),
				QCom:     item.Quantity.DBString(),
				VUnCom:   item.UnitPrice.DBString(),
				VProd:    item.GrossValue.DBString(),
				CEANTrib: "SEM GTIN",
				UTrib:    strings.ToUpper(strings.TrimSpace(item.Unit)),
				QTrib:    item.Quantity.DBString(),
				VUnTrib:  item.UnitPrice.DBString(),
				VDesc:    discount,
				IndTot:   1,
			},
			Imposto: tax,
		})
	}

	payments := make([]paymentDetailXML, 0, len(input.Payments))
	var paid platform.Money
	for _, payment := range input.Payments {
		code, ok := supportedPaymentCode(payment.Method)
		if !ok {
			return nil, fmt.Errorf("payment method %q is not supported by the conservative NFC-e builder", payment.Method)
		}
		if payment.Amount <= 0 {
			return nil, fmt.Errorf("payment amount must be positive")
		}
		var err error
		paid, err = paid.AddChecked(payment.Amount)
		if err != nil {
			return nil, err
		}
		payments = append(payments, paymentDetailXML{TPag: code, VPag: payment.Amount.DBString()})
	}
	if paid != netTotal {
		return nil, fmt.Errorf("payment total %s does not match NFC-e total %s", paid.DBString(), netTotal.DBString())
	}

	doc := nfeDocumentXML{
		Xmlns: NFeNamespace,
		InfNFe: infNFeXML{
			ID:      "NFe" + input.Reservation.AccessKey,
			Version: Version400,
			Ide: ideXML{
				CUF:      cUF,
				CNF:      input.Reservation.NumericCode,
				NatOp:    "VENDA",
				Mod:      "65",
				Serie:    input.Reservation.Series,
				NNF:      input.Reservation.DocumentNumber,
				DhEmi:    input.Reservation.IssuedAt.Format("2006-01-02T15:04:05-07:00"),
				TpNF:     1,
				IdDest:   1,
				CMunFG:   input.Issuer.AddressCityCode,
				TpImp:    4,
				TpEmis:   input.Reservation.EmissionType,
				CDV:      input.Reservation.CheckDigit,
				TpAmb:    ambient,
				FinNFe:   1,
				IndFinal: 1,
				IndPres:  1,
				ProcEmi:  0,
				VerProc:  strings.TrimSpace(input.ProcessVersion),
			},
			Emit: emitXML{
				CNPJ:  normalizedCNPJ(input.Issuer.CNPJ),
				XNome: strings.TrimSpace(input.Issuer.LegalName),
				XFant: normalizedOptional(input.Issuer.TradeName),
				EnderEmit: emitAddressXML{
					XLgr:    strings.TrimSpace(input.Issuer.AddressStreet),
					Nro:     strings.TrimSpace(input.Issuer.AddressNumber),
					XCpl:    normalizedOptional(input.Issuer.AddressComplement),
					XBairro: strings.TrimSpace(input.Issuer.AddressNeighborhood),
					CMun:    input.Issuer.AddressCityCode,
					XMun:    strings.TrimSpace(input.Issuer.AddressCity),
					UF:      strings.ToUpper(strings.TrimSpace(input.Issuer.AddressState)),
					CEP:     strings.TrimSpace(input.Issuer.AddressZIP),
					CPais:   "1058",
					XPais:   "BRASIL",
				},
				IE:  strings.TrimSpace(input.Issuer.IE),
				CRT: strings.TrimSpace(input.Issuer.CRT),
			},
			Det: items,
			Total: totalXML{
				ICMSTot: icmsTotalXML{
					VBC: "0.00", VICMS: "0.00", VICMSDeson: "0.00", VFCP: "0.00",
					VBCST: "0.00", VST: "0.00", VFCPST: "0.00", VFCPSTRet: "0.00",
					VProd: productTotal.DBString(), VFrete: "0.00", VSeg: "0.00",
					VDesc: discountTotal.DBString(), VII: "0.00", VIPI: "0.00",
					VIPIDevol: "0.00", VPIS: "0.00", VCOFINS: "0.00", VOutro: "0.00",
					VNF: netTotal.DBString(),
				},
				IBSCBSTot: rtcTotals.XML(),
			},
			Transp: transportXML{ModFrete: 9},
			Pag:    paymentGroupXML{Details: payments},
		},
		InfNFeSupl: infNFeSuplXML{QRCode: qrCode, URLChave: consultationURL},
	}

	payload, err := xml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), payload...), nil
}

func validateUnsignedNFCeInput(input UnsignedNFCeInput) error {
	r := input.Reservation
	if r.Status != fisc.NFCeStatusReserved || r.Model != fisc.NFCeModel ||
		r.EmissionType != fisc.NFCeNormalEmissionType || r.IssuedAt.IsZero() {
		return fmt.Errorf("reservation is not a normal reserved NFC-e")
	}
	if err := fisc.ValidateNFCeAccessKey(r.AccessKey); err != nil {
		return fmt.Errorf("reservation access key: %w", err)
	}
	if err := fisc.ValidateCNPJ(input.Issuer.CNPJ); err != nil {
		return fmt.Errorf("issuer CNPJ: %w", err)
	}
	if strings.TrimSpace(input.ProcessVersion) == "" || len(input.ProcessVersion) > 20 {
		return fmt.Errorf("process version is required and must have at most 20 characters")
	}
	if len(input.Items) == 0 || len(input.Payments) == 0 {
		return fmt.Errorf("items and payments are required")
	}
	for expected, item := range input.Items {
		if item.Number != expected+1 {
			return fmt.Errorf("item numbering must be contiguous starting at 1")
		}
		if strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Description) == "" {
			return fmt.Errorf("item %d code and description are required", item.Number)
		}
		if len(strings.TrimSpace(item.NCM)) != 8 || !allDigits(strings.TrimSpace(item.NCM)) {
			return fmt.Errorf("item %d NCM must contain 8 digits", item.Number)
		}
		if item.CEST != nil {
			value := strings.TrimSpace(*item.CEST)
			if value != "" && (len(value) != 7 || !allDigits(value)) {
				return fmt.Errorf("item %d CEST must contain 7 digits", item.Number)
			}
		}
		cfop := strings.TrimSpace(item.CFOP)
		if len(cfop) != 4 || !allDigits(cfop) || cfop[0] != '5' {
			return fmt.Errorf("item %d requires internal-sale CFOP 5xxx in this builder", item.Number)
		}
		unit := strings.TrimSpace(item.Unit)
		if unit == "" || len(unit) > 6 || item.Quantity <= 0 || item.UnitPrice <= 0 ||
			item.GrossValue <= 0 || item.DiscountValue < 0 {
			return fmt.Errorf("item %d has invalid commercial values", item.Number)
		}
		if item.Tax.RTCTax.IS != nil {
			return fmt.Errorf("item %d contains selective tax; IS XML is not enabled for the 2026 NFC-e profile", item.Number)
		}
	}
	return nil
}

func buildLegacyTaxXML(calculation fisc.InvoiceItemTaxCalculation) (taxXML, error) {
	legacy := calculation.LegacyTax
	var icms icmsXML
	switch legacy.ICMS.Regime {
	case "csosn":
		switch legacy.ICMS.Code {
		case "102", "103", "300", "400":
			icms.ICMSSN102 = &icmsSN102XML{Orig: legacy.ICMS.Origin, CSOSN: legacy.ICMS.Code}
		default:
			return taxXML{}, fmt.Errorf("CSOSN %s requires tax fields not modeled yet", legacy.ICMS.Code)
		}
	case "cst":
		switch legacy.ICMS.Code {
		case "40", "41", "50":
			icms.ICMS40 = &icms40XML{Orig: legacy.ICMS.Origin, CST: legacy.ICMS.Code}
		default:
			return taxXML{}, fmt.Errorf("ICMS CST %s requires tax fields not modeled yet", legacy.ICMS.Code)
		}
	default:
		return taxXML{}, fmt.Errorf("unsupported ICMS regime %q", legacy.ICMS.Regime)
	}

	if !isContributionNT(legacy.PIS.CST) {
		return taxXML{}, fmt.Errorf("PIS CST %s requires tax fields not modeled yet", legacy.PIS.CST)
	}
	if !isContributionNT(legacy.COFINS.CST) {
		return taxXML{}, fmt.Errorf("COFINS CST %s requires tax fields not modeled yet", legacy.COFINS.CST)
	}
	return taxXML{
		ICMS:   icms,
		PIS:    pisXML{PISNT: &contributionNTXML{CST: legacy.PIS.CST}},
		COFINS: cofinsXML{COFINSNT: &contributionNTXML{CST: legacy.COFINS.CST}},
	}, nil
}

func isContributionNT(cst string) bool {
	switch strings.TrimSpace(cst) {
	case "04", "05", "06", "07", "08", "09":
		return true
	default:
		return false
	}
}

func supportedPaymentCode(method string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "cash":
		return "01", true
	case "pix":
		return "17", true
	case "transfer":
		return "18", true
	default:
		return "", false
	}
}

func normalizedOptional(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func normalizedCNPJ(value string) string {
	var out strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func validatePublicFiscalURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("URL must use http or https")
	}
	return parsed.String(), nil
}
