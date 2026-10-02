package sefaz

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
	"github.com/example/sistemaemgo/internal/platform"
)

func unsignedLegacyFixture(t *testing.T) UnsignedNFCeInput {
	t.Helper()
	issuedAt := time.Date(2026, time.October, 1, 0, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	key, err := fisc.BuildNFCeAccessKey(fisc.NFCeAccessKeyInput{
		UF:           "MG",
		IssuedAt:     issuedAt,
		CNPJ:         "12.ABC.345/01DE-35",
		Series:       1,
		Number:       42,
		NumericCode:  "12345678",
		EmissionType: fisc.NFCeNormalEmissionType,
	})
	if err != nil {
		t.Fatal(err)
	}
	return UnsignedNFCeInput{
		Reservation: fisc.NFCeReservation{
			Status:         fisc.NFCeStatusReserved,
			Model:          fisc.NFCeModel,
			Series:         1,
			DocumentNumber: 42,
			Environment:    "homologation",
			AccessKey:      key,
			EmissionType:   fisc.NFCeNormalEmissionType,
			NumericCode:    "12345678",
			CheckDigit:     int(key[len(key)-1] - '0'),
			IssuedAt:       issuedAt,
		},
		Issuer: fisc.NFCeIssuerProfile{
			LegalName:           "Empresa Teste LTDA",
			TradeName:           stringPtr("Loja Teste"),
			CNPJ:                "12.ABC.345/01DE-35",
			IE:                  "123456789",
			CRT:                 "1",
			AddressStreet:       "Rua Teste",
			AddressNumber:       "100",
			AddressNeighborhood: "Centro",
			AddressCity:         "Uberlandia",
			AddressCityCode:     "3170206",
			AddressState:        "MG",
			AddressZIP:          "38400000",
		},
		Items: []UnsignedNFCeItem{{
			Number:        1,
			Code:          "SKU-1",
			Description:   "Produto teste",
			Unit:          "UN",
			NCM:           "22021000",
			CFOP:          "5102",
			Quantity:      platform.NewQuantityMilli(1000),
			UnitPrice:     platform.NewMoneyCents(1000),
			GrossValue:    platform.NewMoneyCents(1000),
			DiscountValue: platform.NewMoneyCents(100),
			Tax: fisc.InvoiceItemTaxCalculation{
				LegacyTax: fisc.LegacyTaxCalculation{
					ICMS:   fisc.LegacyICMSTax{Origin: "0", Regime: "csosn", Code: "102"},
					PIS:    fisc.LegacyContributionTax{CST: "07"},
					COFINS: fisc.LegacyContributionTax{CST: "07"},
				},
			},
		}},
		Payments:        []UnsignedNFCePayment{{Method: "cash", Amount: platform.NewMoneyCents(900)}},
		ProcessVersion:  "SistemaEmGo-2026.10",
		QRCodeBaseURL:   "https://nfce.example.test/qrcode",
		ConsultationURL: "https://nfce.example.test/consulta",
	}
}

func TestBuildUnsignedNFCeLegacyCandidate(t *testing.T) {
	input := unsignedLegacyFixture(t)
	content, err := BuildUnsignedNFCeLegacyCandidate(input)
	if err != nil {
		t.Fatalf("BuildUnsignedNFCeLegacyCandidate: %v", err)
	}
	var root struct {
		XMLName xml.Name `xml:"NFe"`
		InfNFe  struct {
			ID  string `xml:"Id,attr"`
			Ide struct {
				Mod   string `xml:"mod"`
				TpAmb string `xml:"tpAmb"`
			} `xml:"ide"`
			Det []struct {
				Prod struct {
					XProd string `xml:"xProd"`
					CFOP  string `xml:"CFOP"`
					VDesc string `xml:"vDesc"`
				} `xml:"prod"`
				Imposto struct {
					ICMS struct {
						SN102 struct {
							CSOSN string `xml:"CSOSN"`
						} `xml:"ICMSSN102"`
					} `xml:"ICMS"`
					PIS struct {
						NT struct {
							CST string `xml:"CST"`
						} `xml:"PISNT"`
					} `xml:"PIS"`
				} `xml:"imposto"`
			} `xml:"det"`
			Total struct {
				ICMSTot struct {
					VProd string `xml:"vProd"`
					VDesc string `xml:"vDesc"`
					VNF   string `xml:"vNF"`
				} `xml:"ICMSTot"`
			} `xml:"total"`
			Pag struct {
				Det []struct {
					TPag string `xml:"tPag"`
					VPag string `xml:"vPag"`
				} `xml:"detPag"`
			} `xml:"pag"`
		} `xml:"infNFe"`
		Supl struct {
			QR string `xml:"qrCode"`
		} `xml:"infNFeSupl"`
	}
	if err := xml.Unmarshal(content, &root); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, content)
	}
	if root.InfNFe.ID != "NFe"+input.Reservation.AccessKey {
		t.Fatalf("Id=%s", root.InfNFe.ID)
	}
	if root.InfNFe.Ide.Mod != "65" || root.InfNFe.Ide.TpAmb != "2" {
		t.Fatalf("unexpected ide: %+v", root.InfNFe.Ide)
	}
	if len(root.InfNFe.Det) != 1 {
		t.Fatalf("det count=%d", len(root.InfNFe.Det))
	}
	item := root.InfNFe.Det[0]
	if item.Prod.XProd != homologationProductDescription {
		t.Fatalf("homologation xProd=%q", item.Prod.XProd)
	}
	if item.Prod.CFOP != "5102" || item.Prod.VDesc != "1.00" {
		t.Fatalf("unexpected product: %+v", item.Prod)
	}
	if item.Imposto.ICMS.SN102.CSOSN != "102" || item.Imposto.PIS.NT.CST != "07" {
		t.Fatalf("unexpected taxes: %+v", item.Imposto)
	}
	if root.InfNFe.Total.ICMSTot.VProd != "10.00" ||
		root.InfNFe.Total.ICMSTot.VDesc != "1.00" ||
		root.InfNFe.Total.ICMSTot.VNF != "9.00" {
		t.Fatalf("unexpected totals: %+v", root.InfNFe.Total.ICMSTot)
	}
	if len(root.InfNFe.Pag.Det) != 1 ||
		root.InfNFe.Pag.Det[0].TPag != "01" ||
		root.InfNFe.Pag.Det[0].VPag != "9.00" {
		t.Fatalf("unexpected payments: %+v", root.InfNFe.Pag.Det)
	}
	if !strings.Contains(root.Supl.QR, "|3|2") {
		t.Fatalf("QR Code v3 missing: %s", root.Supl.QR)
	}
}

func TestBuildUnsignedNFCeLegacyCandidateRejectsRTC(t *testing.T) {
	input := unsignedLegacyFixture(t)
	input.Items[0].Tax.RTCTax.IBSCBS = &fisc.RegularIBSCBSResult{
		CST:            "000",
		Classification: "000001",
		Base:           platform.NewMoneyCents(900),
	}
	if _, err := BuildUnsignedNFCeLegacyCandidate(input); err == nil ||
		!strings.Contains(err.Error(), "blocks RTC XML") {
		t.Fatalf("expected RTC block, got %v", err)
	}
}

func TestBuildUnsignedNFCeLegacyCandidateRejectsUnsupportedPayment(t *testing.T) {
	input := unsignedLegacyFixture(t)
	input.Payments = []UnsignedNFCePayment{{Method: "credit", Amount: platform.NewMoneyCents(900)}}
	if _, err := BuildUnsignedNFCeLegacyCandidate(input); err == nil ||
		!strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected unsupported payment error, got %v", err)
	}
}

func TestBuildUnsignedNFCeLegacyCandidateRejectsTaxedLegacyCodesWithoutRates(t *testing.T) {
	input := unsignedLegacyFixture(t)
	input.Items[0].Tax.LegacyTax.ICMS = fisc.LegacyICMSTax{
		Origin: "0", Regime: "cst", Code: "00",
	}
	if _, err := BuildUnsignedNFCeLegacyCandidate(input); err == nil ||
		!strings.Contains(err.Error(), "requires tax fields") {
		t.Fatalf("expected ICMS rate-field block, got %v", err)
	}
}

func stringPtr(value string) *string { return &value }
