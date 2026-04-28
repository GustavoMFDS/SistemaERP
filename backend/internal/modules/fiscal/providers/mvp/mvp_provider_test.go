package mvp

import (
	"context"
	"encoding/xml"
	"testing"

	inv "github.com/example/sistemaemgo/internal/modules/inventory/domain"
	sales "github.com/example/sistemaemgo/internal/modules/sales/domain"
)

type nfeDoc struct {
	XMLName xml.Name `xml:"NFe"`
	InfNFe  struct {
		ID     string `xml:"Id,attr"`
		Versao string `xml:"versao,attr"`
		Det    []struct {
			NItem string `xml:"nItem,attr"`
			Prod  struct {
				CProd  string `xml:"cProd"`
				XProd  string `xml:"xProd"`
				CEAN   string `xml:"cEAN"`
				UCom   string `xml:"uCom"`
				QCom   string `xml:"qCom"`
				VUnCom string `xml:"vUnCom"`
				VProd  string `xml:"vProd"`
				NCM    string `xml:"NCM"`
				CFOP   string `xml:"CFOP"`
			} `xml:"prod"`
		} `xml:"det"`
		Total struct {
			ICMSTot struct {
				VProd string `xml:"vProd"`
				VDesc string `xml:"vDesc"`
				VNF   string `xml:"vNF"`
			} `xml:"ICMSTot"`
		} `xml:"total"`
	} `xml:"infNFe"`
}

func TestProvider_GenerateNFeXML_OK(t *testing.T) {
	p := New()

	sale := sales.Sale{ID: "sale-1", DiscountValue: 2.00, Total: 18.00}
	items := []sales.SaleItem{{ProductID: "prod-1", Qty: 2, UnitPrice: 10.00}}
	products := map[string]inv.Product{
		"prod-1": {ID: "prod-1", SKU: "P001", Name: "Produto 1", Unit: "UN", Active: true},
	}

	content, fileName, err := p.GenerateNFeXML(context.Background(), sale, items, products)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fileName != "NFe-sale-1.xml" {
		t.Fatalf("want fileName=NFe-sale-1.xml, got %s", fileName)
	}

	var doc nfeDoc
	if err := xml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.InfNFe.ID != "NFe"+sale.ID {
		t.Fatalf("want Id=%s, got %s", "NFe"+sale.ID, doc.InfNFe.ID)
	}
	if doc.InfNFe.Total.ICMSTot.VProd != "20.00" {
		t.Fatalf("want vProd=20.00, got %s", doc.InfNFe.Total.ICMSTot.VProd)
	}
	if doc.InfNFe.Total.ICMSTot.VDesc != "2.00" {
		t.Fatalf("want vDesc=2.00, got %s", doc.InfNFe.Total.ICMSTot.VDesc)
	}
	if doc.InfNFe.Total.ICMSTot.VNF != "18.00" {
		t.Fatalf("want vNF=18.00, got %s", doc.InfNFe.Total.ICMSTot.VNF)
	}
	if len(doc.InfNFe.Det) != 1 {
		t.Fatalf("want 1 det, got %d", len(doc.InfNFe.Det))
	}
	if doc.InfNFe.Det[0].Prod.CProd != "P001" {
		t.Fatalf("want cProd=P001, got %s", doc.InfNFe.Det[0].Prod.CProd)
	}
	if doc.InfNFe.Det[0].Prod.QCom != "2.000" {
		t.Fatalf("want qCom=2.000, got %s", doc.InfNFe.Det[0].Prod.QCom)
	}
	if doc.InfNFe.Det[0].Prod.VProd != "20.00" {
		t.Fatalf("want item vProd=20.00, got %s", doc.InfNFe.Det[0].Prod.VProd)
	}
}

func TestProvider_GenerateNFeXML_MissingProductSnapshot(t *testing.T) {
	p := New()
	_, _, err := p.GenerateNFeXML(context.Background(), sales.Sale{ID: "sale-1"}, []sales.SaleItem{{ProductID: "prod-404", Qty: 1, UnitPrice: 1}}, map[string]inv.Product{})
	if err == nil {
		t.Fatalf("expected error")
	}
}
