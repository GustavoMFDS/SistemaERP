package mvp

import (
	"context"
	"encoding/xml"
	"fmt"
	"time"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
)

// Provider generates an MVP NF-e XML that is well-formed and internally consistent.
// It is NOT a SEFAZ-ready NF-e.
//
// This implementation is intentionally minimal and exists mainly to validate the
// end-to-end flow and storage architecture.
//
// Future providers can be added for:
// - digital signature (A1/A3)
// - SEFAZ transmission + protocol
// - DANFE generation
// - full fiscal fields (CST/CSOSN/ICMS/PIS/COFINS, etc.)
//
// This provider is stateless and safe for concurrent use.
type Provider struct{}

func New() *Provider { return &Provider{} }

type input struct {
	sale     sales.Sale
	items    []sales.SaleItem
	products map[string]inv.Product
}

func (p *Provider) GenerateNFeXML(ctx context.Context, sale sales.Sale, items []sales.SaleItem, products map[string]inv.Product) ([]byte, string, error) {
	_ = ctx
	in := input{sale: sale, items: items, products: products}
	return build(in)
}

func build(in input) ([]byte, string, error) {
	// Minimal structure inspired by NF-e concepts.
	type InfAdic struct {
		InfCpl string `xml:"infCpl"`
	}
	type ICMSTot struct {
		VProd string `xml:"vProd"`
		VDesc string `xml:"vDesc"`
		VNF   string `xml:"vNF"`
	}
	type Total struct {
		ICMSTot ICMSTot `xml:"ICMSTot"`
	}
	type Ide struct {
		NatOp  string `xml:"natOp"`
		Mod    string `xml:"mod"`
		Serie  string `xml:"serie"`
		NNF    string `xml:"nNF"`
		DhEmi  string `xml:"dhEmi"`
		TpNF   string `xml:"tpNF"`
		FinNFe string `xml:"finNFe"`
	}
	type Prod struct {
		CProd  string `xml:"cProd"`
		XProd  string `xml:"xProd"`
		CEAN   string `xml:"cEAN"`
		UCom   string `xml:"uCom"`
		QCom   string `xml:"qCom"`
		VUnCom string `xml:"vUnCom"`
		VProd  string `xml:"vProd"`
		NCM    string `xml:"NCM"`
		CFOP   string `xml:"CFOP"`
	}
	type Det struct {
		NItem string `xml:"nItem,attr"`
		Prod  Prod   `xml:"prod"`
	}
	type InfNFe struct {
		ID      string   `xml:"Id,attr"`
		Versao  string   `xml:"versao,attr"`
		Ide     Ide      `xml:"ide"`
		Det     []Det    `xml:"det"`
		Total   Total    `xml:"total"`
		InfAdic *InfAdic `xml:"infAdic,omitempty"`
	}
	type NFe struct {
		XMLName xml.Name `xml:"NFe"`
		InfNFe  InfNFe   `xml:"infNFe"`
	}

	now := time.Now().Format(time.RFC3339)
	nfe := NFe{InfNFe: InfNFe{
		ID:     "NFe" + in.sale.ID,
		Versao: "4.00",
		Ide: Ide{
			NatOp:  "VENDA",
			Mod:    "55",
			Serie:  "1",
			NNF:    "1",
			DhEmi:  now,
			TpNF:   "1",
			FinNFe: "1",
		},
		InfAdic: &InfAdic{InfCpl: fmt.Sprintf("XML MVP gerado a partir da venda %s", in.sale.ID)},
	}}

	var vProd = sales.Sale{}.Total
	for i, it := range in.items {
		p, ok := in.products[it.ProductID]
		if !ok {
			return nil, "", fmt.Errorf("missing product snapshot for product_id=%s", it.ProductID)
		}
		lineGross, err := it.UnitPrice.MulQtyChecked(it.Qty)
		if err != nil {
			return nil, "", fmt.Errorf("invalid item amount for product_id=%s: %w", it.ProductID, err)
		}
		vProd, err = vProd.AddChecked(lineGross)
		if err != nil {
			return nil, "", fmt.Errorf("fiscal total exceeds supported range: %w", err)
		}

		barcode := "SEM GTIN"
		if p.Barcode != nil {
			barcode = *p.Barcode
		}
		// Defaults (configuráveis no futuro)
		ncm := "00000000"
		cfop := "5102"
		nfe.InfNFe.Det = append(nfe.InfNFe.Det, Det{
			NItem: fmt.Sprintf("%d", i + 1),
			Prod: Prod{
				CProd:  p.SKU,
				XProd:  p.Name,
				CEAN:   barcode,
				UCom:   p.Unit,
				QCom:   it.Qty.DBString(),
				VUnCom: it.UnitPrice.DBString(),
				VProd:  lineGross.DBString(),
				NCM:    ncm,
				CFOP:   cfop,
			},
		})
	}

	nfe.InfNFe.Total = Total{ICMSTot: ICMSTot{
		VProd: vProd.DBString(),
		VDesc: in.sale.DiscountValue.DBString(),
		VNF:   in.sale.Total.DBString(),
	}}

	out, err := xml.MarshalIndent(nfe, "", "  ")
	if err != nil {
		return nil, "", err
	}
	out = append([]byte(xml.Header), out...)
	fileName := fmt.Sprintf("NFe-%s.xml", in.sale.ID)
	return out, fileName, nil
}
