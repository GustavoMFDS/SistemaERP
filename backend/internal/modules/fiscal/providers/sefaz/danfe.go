package sefaz

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"html/template"
	"strconv"
	"strings"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	qrcode "github.com/skip2/go-qrcode"
)

type DANFERenderer struct{}

func NewDANFERenderer() *DANFERenderer {
	return &DANFERenderer{}
}

type danfeNFeXML struct {
	InfNFe     danfeInfNFeXML     `xml:"infNFe"`
	InfNFeSupl danfeInfNFeSuplXML `xml:"infNFeSupl"`
}

type danfeInfNFeXML struct {
	ID    string            `xml:"Id,attr"`
	Ide   danfeIdeXML       `xml:"ide"`
	Emit  danfeEmitXML      `xml:"emit"`
	Dest  *danfeDestXML     `xml:"dest"`
	Det   []danfeDetXML     `xml:"det"`
	Total danfeTotalXML     `xml:"total"`
	Pag   danfePaymentGroup `xml:"pag"`
}

type danfeIdeXML struct {
	Serie  int    `xml:"serie"`
	NNF    int64  `xml:"nNF"`
	DhEmi  string `xml:"dhEmi"`
	DhCont string `xml:"dhCont"`
	XJust  string `xml:"xJust"`
	TpEmis int    `xml:"tpEmis"`
	TpAmb  string `xml:"tpAmb"`
}

type danfeEmitXML struct {
	CNPJ      string              `xml:"CNPJ"`
	XNome     string              `xml:"xNome"`
	XFant     string              `xml:"xFant"`
	EnderEmit danfeEmitAddressXML `xml:"enderEmit"`
	IE        string              `xml:"IE"`
}

type danfeEmitAddressXML struct {
	XLgr    string `xml:"xLgr"`
	Nro     string `xml:"nro"`
	XCpl    string `xml:"xCpl"`
	XBairro string `xml:"xBairro"`
	XMun    string `xml:"xMun"`
	UF      string `xml:"UF"`
	CEP     string `xml:"CEP"`
}

type danfeDestXML struct {
	CNPJ      string               `xml:"CNPJ"`
	CPF       string               `xml:"CPF"`
	IDEstrang string               `xml:"idEstrangeiro"`
	XNome     string               `xml:"xNome"`
	EnderDest *danfeDestAddressXML `xml:"enderDest"`
}

type danfeDestAddressXML struct {
	XLgr    string `xml:"xLgr"`
	Nro     string `xml:"nro"`
	XCpl    string `xml:"xCpl"`
	XBairro string `xml:"xBairro"`
	XMun    string `xml:"xMun"`
	UF      string `xml:"UF"`
}

type danfeDetXML struct {
	NItem int             `xml:"nItem,attr"`
	Prod  danfeProductXML `xml:"prod"`
}

type danfeProductXML struct {
	CProd  string `xml:"cProd"`
	XProd  string `xml:"xProd"`
	QCom   string `xml:"qCom"`
	UCom   string `xml:"uCom"`
	VUnCom string `xml:"vUnCom"`
	VProd  string `xml:"vProd"`
	VDesc  string `xml:"vDesc"`
}

type danfeTotalXML struct {
	ICMSTot danfeICMSTotalXML `xml:"ICMSTot"`
}

type danfeICMSTotalXML struct {
	VProd  string `xml:"vProd"`
	VFrete string `xml:"vFrete"`
	VSeg   string `xml:"vSeg"`
	VDesc  string `xml:"vDesc"`
	VOutro string `xml:"vOutro"`
	VNF    string `xml:"vNF"`
}

type danfePaymentGroup struct {
	Details []danfePaymentXML `xml:"detPag"`
	Troco   string            `xml:"vTroco"`
}

type danfePaymentXML struct {
	Method string `xml:"tPag"`
	Amount string `xml:"vPag"`
}

type danfeInfNFeSuplXML struct {
	QRCode   string `xml:"qrCode"`
	URLChave string `xml:"urlChave"`
}

type danfeView struct {
	IssuerName       string
	IssuerTradeName  string
	IssuerCNPJ       string
	IssuerIE         string
	IssuerAddress    string
	Homologation        bool
	Cancelled           bool
	OfflineContingency  bool
	PendingAuthorization bool
	ContingencyStartedAt string
	ContingencyReason    string
	Items               []danfeItemView
	ItemCount        int
	ProductTotal     string
	Freight          string
	Insurance        string
	Other            string
	Discount         string
	AmountDue        string
	Payments         []danfePaymentView
	Change           string
	Number           int64
	Series           int
	IssuedAt         string
	AccessKey        string
	AccessKeyGrouped string
	ConsultationURL  string
	Protocol         string
	AuthorizedAt     string
	Consumer         danfeConsumerView
	QRCodeDataURL    template.URL
}

type danfeItemView struct {
	Code        string
	Description string
	Quantity    string
	Unit        string
	UnitPrice   string
	Total       string
	Discount    string
}

type danfePaymentView struct {
	Method string
	Amount string
}

type danfeConsumerView struct {
	Identified bool
	Document   string
	Name       string
	Address    string
}

func (r *DANFERenderer) Render(
	reservation fisc.NFCeReservation,
	signedXML []byte,
) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("DANFE renderer is not configured")
	}
	offlinePending := reservation.Status == fisc.NFCeStatusSigned &&
		reservation.EmissionType == fisc.NFCeOfflineContingencyEmissionType
	if reservation.Status != fisc.NFCeStatusAuthorized &&
		reservation.Status != fisc.NFCeStatusCancelled &&
		!offlinePending {
		return nil, fmt.Errorf("DANFE requires an authorized/cancelled NFC-e or a signed offline contingency")
	}
	if offlinePending {
		if reservation.ContingencyStartedAt == nil ||
			reservation.ContingencyStartedAt.IsZero() ||
			reservation.ContingencyJustification == nil ||
			strings.TrimSpace(*reservation.ContingencyJustification) == "" {
			return nil, fmt.Errorf("offline contingency metadata is incomplete")
		}
	} else if reservation.AuthorizationProtocol == nil ||
		strings.TrimSpace(*reservation.AuthorizationProtocol) == "" ||
		reservation.AuthorizedAt == nil ||
		reservation.AuthorizedAt.IsZero() {
		return nil, fmt.Errorf("NFC-e authorization metadata is incomplete")
	}
	if len(signedXML) == 0 {
		return nil, fmt.Errorf("signed NFC-e XML is empty")
	}

	var doc danfeNFeXML
	if err := xml.Unmarshal(signedXML, &doc); err != nil {
		return nil, fmt.Errorf("parse signed NFC-e XML: %w", err)
	}
	if strings.TrimSpace(doc.InfNFe.ID) != "NFe"+reservation.AccessKey {
		return nil, fmt.Errorf("DANFE XML access key does not match reservation")
	}
	if doc.InfNFe.Ide.NNF != reservation.DocumentNumber ||
		doc.InfNFe.Ide.Serie != reservation.Series ||
		doc.InfNFe.Ide.TpEmis != reservation.EmissionType {
		return nil, fmt.Errorf("DANFE XML document identity does not match reservation")
	}
	qrURL := strings.TrimSpace(doc.InfNFeSupl.QRCode)
	consultURL := strings.TrimSpace(doc.InfNFeSupl.URLChave)
	if qrURL == "" || consultURL == "" {
		return nil, fmt.Errorf("DANFE requires qrCode and urlChave from the NFC-e XML")
	}
	qrPNG, err := qrcode.Encode(qrURL, qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("encode DANFE QR Code: %w", err)
	}

	items := make([]danfeItemView, 0, len(doc.InfNFe.Det))
	for _, item := range doc.InfNFe.Det {
		items = append(items, danfeItemView{
			Code:        strings.TrimSpace(item.Prod.CProd),
			Description: strings.TrimSpace(item.Prod.XProd),
			Quantity:    formatDecimalBR(item.Prod.QCom),
			Unit:        strings.TrimSpace(item.Prod.UCom),
			UnitPrice:   formatDecimalBR(item.Prod.VUnCom),
			Total:       formatDecimalBR(item.Prod.VProd),
			Discount:    optionalMoneyBR(item.Prod.VDesc),
		})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("DANFE requires at least one item")
	}

	payments := make([]danfePaymentView, 0, len(doc.InfNFe.Pag.Details))
	for _, payment := range doc.InfNFe.Pag.Details {
		payments = append(payments, danfePaymentView{
			Method: paymentMethodLabel(payment.Method),
			Amount: formatMoneyBR(payment.Amount),
		})
	}

	protocol := ""
	authorizedAt := ""
	if reservation.AuthorizationProtocol != nil {
		protocol = strings.TrimSpace(*reservation.AuthorizationProtocol)
	}
	if reservation.AuthorizedAt != nil && !reservation.AuthorizedAt.IsZero() {
		authorizedAt = reservation.AuthorizedAt.Format("02/01/2006 15:04:05 -07:00")
	}
	contingencyStartedAt := ""
	contingencyReason := ""
	if reservation.ContingencyStartedAt != nil {
		contingencyStartedAt = reservation.ContingencyStartedAt.Format("02/01/2006 15:04:05 -07:00")
	}
	if reservation.ContingencyJustification != nil {
		contingencyReason = strings.TrimSpace(*reservation.ContingencyJustification)
	}

	view := danfeView{
		IssuerName:       strings.TrimSpace(doc.InfNFe.Emit.XNome),
		IssuerTradeName:  strings.TrimSpace(doc.InfNFe.Emit.XFant),
		IssuerCNPJ:       formatCNPJ(doc.InfNFe.Emit.CNPJ),
		IssuerIE:         strings.TrimSpace(doc.InfNFe.Emit.IE),
		IssuerAddress:    formatIssuerAddress(doc.InfNFe.Emit.EnderEmit),
		Homologation:        reservation.Environment == "homologation" || doc.InfNFe.Ide.TpAmb == "2",
		Cancelled:           reservation.Status == fisc.NFCeStatusCancelled,
		OfflineContingency: reservation.EmissionType == fisc.NFCeOfflineContingencyEmissionType,
		PendingAuthorization: offlinePending,
		ContingencyStartedAt: contingencyStartedAt,
		ContingencyReason:    contingencyReason,
		Items:               items,
		ItemCount:        len(items),
		ProductTotal:     formatMoneyBR(doc.InfNFe.Total.ICMSTot.VProd),
		Freight:          optionalMoneyBR(doc.InfNFe.Total.ICMSTot.VFrete),
		Insurance:        optionalMoneyBR(doc.InfNFe.Total.ICMSTot.VSeg),
		Other:            optionalMoneyBR(doc.InfNFe.Total.ICMSTot.VOutro),
		Discount:         optionalMoneyBR(doc.InfNFe.Total.ICMSTot.VDesc),
		AmountDue:        formatMoneyBR(doc.InfNFe.Total.ICMSTot.VNF),
		Payments:         payments,
		Change:           optionalMoneyBR(doc.InfNFe.Pag.Troco),
		Number:           reservation.DocumentNumber,
		Series:           reservation.Series,
		IssuedAt:         formatFiscalTimestamp(doc.InfNFe.Ide.DhEmi),
		AccessKey:        reservation.AccessKey,
		AccessKeyGrouped: groupAccessKey(reservation.AccessKey),
		ConsultationURL:  consultURL,
		Protocol:         protocol,
		AuthorizedAt:     authorizedAt,
		Consumer:         danfeConsumer(doc.InfNFe.Dest),
		QRCodeDataURL:    template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(qrPNG)),
	}

	var out bytes.Buffer
	if err := danfeTemplate.Execute(&out, view); err != nil {
		return nil, fmt.Errorf("render DANFE NFC-e: %w", err)
	}
	return out.Bytes(), nil
}

var danfeTemplate = template.Must(template.New("danfe-nfce").Parse(`<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data:; style-src 'unsafe-inline'">
<title>DANFE NFC-e {{.Number}}</title>
<style>
@page { margin: 2mm; }
* { box-sizing: border-box; }
body { width: 80mm; min-width: 52mm; margin: 0 auto; padding: 2mm; font: 11px/1.25 Arial, Helvetica, sans-serif; color: #000; background: #fff; }
h1,p { margin: 0; }
.center { text-align: center; }
.strong { font-weight: 700; }
.section { border-top: 1px dashed #000; padding-top: 2mm; margin-top: 2mm; }
.banner { border: 2px solid #000; padding: 2mm; margin: 2mm 0; font-weight: 700; text-align: center; }
table { width: 100%; border-collapse: collapse; }
th, td { padding: 1mm 0.5mm; vertical-align: top; }
th { border-bottom: 1px solid #000; font-size: 9px; }
td.num, th.num { text-align: right; white-space: nowrap; }
.item-desc { word-break: break-word; }
.row { display: flex; justify-content: space-between; gap: 3mm; }
.qr { text-align: center; }
.qr img { width: 32mm; height: 32mm; image-rendering: pixelated; }
.key { word-break: break-word; font-family: monospace; font-size: 10px; }
.muted { font-size: 9px; }
@media print {
  body { width: 80mm; margin: 0 auto; padding: 0 2mm; }
  .no-print { display: none; }
}
</style>
</head>
<body>
<header class="center">
  <div class="strong">{{.IssuerName}}</div>
  {{if .IssuerTradeName}}<div>{{.IssuerTradeName}}</div>{{end}}
  <div>CNPJ {{.IssuerCNPJ}}{{if .IssuerIE}} · IE {{.IssuerIE}}{{end}}</div>
  <div>{{.IssuerAddress}}</div>
  <div class="section strong">DOCUMENTO AUXILIAR DA NOTA FISCAL DE CONSUMIDOR ELETRÔNICA</div>
</header>

{{if .Homologation}}<div class="banner">EMITIDA EM AMBIENTE DE HOMOLOGAÇÃO — SEM VALOR FISCAL</div>{{end}}
{{if .OfflineContingency}}<div class="banner">EMITIDA EM CONTINGÊNCIA</div>{{end}}
{{if .PendingAuthorization}}<div class="banner">PENDENTE DE TRANSMISSÃO / AUTORIZAÇÃO SEFAZ</div>{{end}}
{{if .Cancelled}}<div class="banner">NFC-e CANCELADA</div>{{end}}

<section class="section">
<table>
<thead>
<tr><th>CÓDIGO / DESCRIÇÃO</th><th class="num">QTD UN × VL UNIT</th><th class="num">VL TOTAL</th></tr>
</thead>
<tbody>
{{range .Items}}
<tr>
<td class="item-desc">{{.Code}} · {{.Description}}{{if .Discount}}<div class="muted">Desconto: R$ {{.Discount}}</div>{{end}}</td>
<td class="num">{{.Quantity}} {{.Unit}} × {{.UnitPrice}}</td>
<td class="num">{{.Total}}</td>
</tr>
{{end}}
</tbody>
</table>
</section>

<section class="section">
<div class="row"><span>Qtde. total de itens</span><strong>{{.ItemCount}}</strong></div>
<div class="row"><span>Valor total R$</span><strong>{{.ProductTotal}}</strong></div>
{{if .Freight}}<div class="row"><span>Frete R$</span><strong>{{.Freight}}</strong></div>{{end}}
{{if .Insurance}}<div class="row"><span>Seguro R$</span><strong>{{.Insurance}}</strong></div>{{end}}
{{if .Other}}<div class="row"><span>Outras despesas R$</span><strong>{{.Other}}</strong></div>{{end}}
{{if .Discount}}<div class="row"><span>Desconto R$</span><strong>{{.Discount}}</strong></div>{{end}}
<div class="row strong"><span>VALOR A PAGAR R$</span><span>{{.AmountDue}}</span></div>
{{range .Payments}}<div class="row"><span>{{.Method}}</span><span>{{.Amount}}</span></div>{{end}}
{{if .Change}}<div class="row"><span>Troco R$</span><span>{{.Change}}</span></div>{{end}}
</section>

<section class="section center">
<div>Número {{.Number}} · Série {{.Series}}</div>
<div>Emissão {{.IssuedAt}}</div>
<div class="muted">Consulte pela chave de acesso em</div>
<div>{{.ConsultationURL}}</div>
<div class="key">{{.AccessKeyGrouped}}</div>
{{if .OfflineContingency}}
<div>Entrada em contingência: {{.ContingencyStartedAt}}</div>
<div class="muted">Motivo: {{.ContingencyReason}}</div>
{{end}}
{{if .Protocol}}
<div>Protocolo de Autorização: {{.Protocol}}</div>
<div>{{.AuthorizedAt}}</div>
{{else if .PendingAuthorization}}
<div class="strong">Sem protocolo — transmissão pendente</div>
{{end}}
</section>

<section class="section center">
{{if .Consumer.Identified}}
<div class="strong">CONSUMIDOR</div>
<div>{{.Consumer.Document}}{{if .Consumer.Name}} · {{.Consumer.Name}}{{end}}</div>
{{if .Consumer.Address}}<div>{{.Consumer.Address}}</div>{{end}}
{{else}}
<div class="strong">CONSUMIDOR NÃO IDENTIFICADO</div>
{{end}}
</section>

<section class="section qr">
<img src="{{.QRCodeDataURL}}" alt="QR Code de consulta da NFC-e">
<div class="muted">Consulte a NFC-e pela leitura do QR Code</div>
</section>
</body>
</html>`))

func formatIssuerAddress(a danfeEmitAddressXML) string {
	parts := []string{strings.TrimSpace(a.XLgr)}
	if v := strings.TrimSpace(a.Nro); v != "" {
		parts[0] += ", " + v
	}
	for _, v := range []string{a.XCpl, a.XBairro, a.XMun, a.UF} {
		if v = strings.TrimSpace(v); v != "" {
			parts = append(parts, v)
		}
	}
	if cep := formatCEP(a.CEP); cep != "" {
		parts = append(parts, "CEP "+cep)
	}
	return strings.Join(parts, " · ")
}

func danfeConsumer(dest *danfeDestXML) danfeConsumerView {
	if dest == nil {
		return danfeConsumerView{}
	}
	doc := ""
	switch {
	case strings.TrimSpace(dest.CNPJ) != "":
		doc = "CNPJ " + formatCNPJ(dest.CNPJ)
	case strings.TrimSpace(dest.CPF) != "":
		doc = "CPF " + formatCPF(dest.CPF)
	case strings.TrimSpace(dest.IDEstrang) != "":
		doc = "ID Estrangeiro " + strings.TrimSpace(dest.IDEstrang)
	}
	out := danfeConsumerView{
		Identified: doc != "" || strings.TrimSpace(dest.XNome) != "",
		Document:   doc,
		Name:       strings.TrimSpace(dest.XNome),
	}
	if dest.EnderDest != nil {
		a := dest.EnderDest
		parts := []string{strings.TrimSpace(a.XLgr)}
		if v := strings.TrimSpace(a.Nro); v != "" {
			parts[0] += ", " + v
		}
		for _, v := range []string{a.XCpl, a.XBairro, a.XMun, a.UF} {
			if v = strings.TrimSpace(v); v != "" {
				parts = append(parts, v)
			}
		}
		out.Address = strings.Join(parts, " · ")
	}
	return out
}

func paymentMethodLabel(code string) string {
	switch strings.TrimSpace(code) {
	case "01":
		return "Dinheiro"
	case "02":
		return "Cheque"
	case "03":
		return "Cartão de crédito"
	case "04":
		return "Cartão de débito"
	case "17":
		return "PIX"
	case "18":
		return "Transferência bancária"
	case "90":
		return "Sem pagamento"
	default:
		if strings.TrimSpace(code) == "" {
			return "Forma de pagamento"
		}
		return "Forma de pagamento " + strings.TrimSpace(code)
	}
}

func formatFiscalTimestamp(raw string) string {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return t.Format("02/01/2006 15:04:05 -07:00")
}

func formatMoneyBR(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "0,00"
	}
	return formatDecimalBR(raw)
}

func optionalMoneyBR(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err == nil && value == 0 {
		return ""
	}
	return formatDecimalBR(raw)
}

func formatDecimalBR(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	sign := ""
	if strings.HasPrefix(raw, "-") {
		sign = "-"
		raw = strings.TrimPrefix(raw, "-")
	}
	parts := strings.SplitN(raw, ".", 2)
	integer := parts[0]
	if integer == "" {
		integer = "0"
	}
	var grouped []string
	for len(integer) > 3 {
		grouped = append([]string{integer[len(integer)-3:]}, grouped...)
		integer = integer[:len(integer)-3]
	}
	grouped = append([]string{integer}, grouped...)
	result := sign + strings.Join(grouped, ".")
	if len(parts) == 2 && parts[1] != "" {
		result += "," + parts[1]
	}
	return result
}

func groupAccessKey(key string) string {
	key = strings.TrimSpace(key)
	var parts []string
	for len(key) > 4 {
		parts = append(parts, key[:4])
		key = key[4:]
	}
	if key != "" {
		parts = append(parts, key)
	}
	return strings.Join(parts, " ")
}

func formatCNPJ(raw string) string {
	value := digits(raw)
	if len(value) != 14 {
		return strings.TrimSpace(raw)
	}
	return value[:2] + "." + value[2:5] + "." + value[5:8] + "/" + value[8:12] + "-" + value[12:]
}

func formatCPF(raw string) string {
	value := digits(raw)
	if len(value) != 11 {
		return strings.TrimSpace(raw)
	}
	return value[:3] + "." + value[3:6] + "." + value[6:9] + "-" + value[9:]
}

func formatCEP(raw string) string {
	value := digits(raw)
	if len(value) != 8 {
		return strings.TrimSpace(raw)
	}
	return value[:5] + "-" + value[5:]
}

func digits(raw string) string {
	var out strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			out.WriteRune(r)
		}
	}
	return out.String()
}
