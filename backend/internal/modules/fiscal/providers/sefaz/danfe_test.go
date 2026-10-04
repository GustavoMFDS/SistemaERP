package sefaz

import (
	"strings"
	"testing"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

func TestDANFERendererRendersAuthorizedNFCe(t *testing.T) {
	const accessKey = "31260912345678000195650010000000421123456789"
	protocol := "131260000000001"
	authorizedAt := time.Date(2026, 10, 4, 10, 31, 0, 0, time.FixedZone("BRT", -3*60*60))
	reservation := fisc.NFCeReservation{
		Status:                fisc.NFCeStatusAuthorized,
		Series:                1,
		DocumentNumber:        42,
		Environment:           "homologation",
		AccessKey:             accessKey,
		AuthorizationProtocol: &protocol,
		AuthorizedAt:          &authorizedAt,
	}
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<NFe xmlns="http://www.portalfiscal.inf.br/nfe">
  <infNFe Id="NFe` + accessKey + `" versao="4.00">
    <ide><serie>1</serie><nNF>42</nNF><dhEmi>2026-10-04T10:30:00-03:00</dhEmi><tpAmb>2</tpAmb></ide>
    <emit>
      <CNPJ>12345678000195</CNPJ><xNome>Empresa Exemplo LTDA</xNome><xFant>Loja Exemplo</xFant>
      <enderEmit><xLgr>Av Fiscal</xLgr><nro>100</nro><xBairro>Centro</xBairro><xMun>Uberlandia</xMun><UF>MG</UF><CEP>38400000</CEP></enderEmit>
      <IE>110042490114</IE>
    </emit>
    <det nItem="1"><prod><cProd>SKU-1</cProd><xProd>Produto &amp; teste</xProd><uCom>UN</uCom><qCom>2.000</qCom><vUnCom>5.00</vUnCom><vProd>10.00</vProd><vDesc>1.00</vDesc></prod></det>
    <total><ICMSTot><vProd>10.00</vProd><vFrete>0.00</vFrete><vSeg>0.00</vSeg><vDesc>1.00</vDesc><vOutro>0.00</vOutro><vNF>9.00</vNF></ICMSTot></total>
    <pag><detPag><tPag>17</tPag><vPag>9.00</vPag></detPag></pag>
  </infNFe>
  <infNFeSupl>
    <qrCode>https://portal.example/nfce/qrcode?p=` + accessKey + `|3|2</qrCode>
    <urlChave>https://portal.example/nfce/consulta</urlChave>
  </infNFeSupl>
</NFe>`)

	html, err := NewDANFERenderer().Render(reservation, xml)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	got := string(html)
	for _, want := range []string{
		"DOCUMENTO AUXILIAR DA NOTA FISCAL DE CONSUMIDOR ELETRÔNICA",
		"EMITIDA EM AMBIENTE DE HOMOLOGAÇÃO",
		"Empresa Exemplo LTDA",
		"12.345.678/0001-95",
		"Produto &amp; teste",
		"2,000 UN × 5,00",
		"VALOR A PAGAR R$",
		"9,00",
		"PIX",
		"CONSUMIDOR NÃO IDENTIFICADO",
		"131260000000001",
		"3126 0912 3456 7800 0195 6500 1000 0000 4211 2345 6789",
		"data:image/png;base64,",
		"width: 32mm",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("DANFE missing %q", want)
		}
	}
	if strings.Contains(got, "#ZgotmplZ") {
		t.Fatal("QR Code data URL was sanitized by html/template")
	}
}

func TestDANFERendererRejectsNonAuthorizedOrMismatchedXML(t *testing.T) {
	const accessKey = "31260912345678000195650010000000421123456789"
	protocol := "131260000000001"
	authorizedAt := time.Now()
	renderer := NewDANFERenderer()

	reservation := fisc.NFCeReservation{
		Status:                fisc.NFCeStatusSigned,
		Series:                1,
		DocumentNumber:        42,
		Environment:           "production",
		AccessKey:             accessKey,
		AuthorizationProtocol: &protocol,
		AuthorizedAt:          &authorizedAt,
	}
	if _, err := renderer.Render(reservation, []byte("<NFe/>")); err == nil {
		t.Fatal("expected non-authorized reservation to be rejected")
	}

	reservation.Status = fisc.NFCeStatusAuthorized
	xml := []byte(`<NFe><infNFe Id="NFe00000000000000000000000000000000000000000000"><ide><serie>1</serie><nNF>42</nNF></ide></infNFe></NFe>`)
	if _, err := renderer.Render(reservation, xml); err == nil ||
		!strings.Contains(err.Error(), "access key") {
		t.Fatalf("expected access-key mismatch, got %v", err)
	}
}
