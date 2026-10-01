package sefaz

import (
	"strings"
	"testing"
)

const testAccessKey = "31260912ABC34501DE35650010000000421123456788"

func TestBuildStatusRequest(t *testing.T) {
	content, err := BuildStatusRequest(EnvironmentHomologation, "MG")
	if err != nil {
		t.Fatalf("BuildStatusRequest: %v", err)
	}
	xml := string(content)
	for _, want := range []string{
		"<consStatServ",
		`versao="4.00"`,
		"<tpAmb>2</tpAmb>",
		"<cUF>31</cUF>",
		"<xServ>STATUS</xServ>",
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("status request missing %q: %s", want, xml)
		}
	}
}

func TestBuildConsultationRequest(t *testing.T) {
	content, err := BuildConsultationRequest(EnvironmentHomologation, testAccessKey)
	if err != nil {
		t.Fatalf("BuildConsultationRequest: %v", err)
	}
	xml := string(content)
	if !strings.Contains(xml, "<chNFe>"+testAccessKey+"</chNFe>") {
		t.Fatalf("consultation access key missing: %s", xml)
	}
	if _, err := BuildConsultationRequest(EnvironmentHomologation, testAccessKey[:43]+"9"); err == nil {
		t.Fatal("expected invalid access-key DV to fail")
	}
}

func TestBuildAuthorizationBatch(t *testing.T) {
	signed := []byte(`<NFe xmlns="http://www.portalfiscal.inf.br/nfe"><infNFe Id="NFe` + testAccessKey + `" versao="4.00"></infNFe><Signature xmlns="http://www.w3.org/2000/09/xmldsig#"></Signature></NFe>`)
	content, err := BuildAuthorizationBatch("42", signed)
	if err != nil {
		t.Fatalf("BuildAuthorizationBatch: %v", err)
	}
	xml := string(content)
	if !strings.Contains(xml, "<idLote>42</idLote>") || !strings.Contains(xml, "<indSinc>1</indSinc>") {
		t.Fatalf("authorization batch metadata missing: %s", xml)
	}
	if !strings.Contains(xml, "<NFe ") {
		t.Fatalf("signed NFe missing from batch: %s", xml)
	}
	if _, err := BuildAuthorizationBatch("ABC", signed); err == nil {
		t.Fatal("expected non-numeric idLote to fail")
	}
	if _, err := BuildAuthorizationBatch("1", []byte("<foo/>")); err == nil {
		t.Fatal("expected wrong signed document root to fail")
	}
}

func TestParseAuthorizationResponseAuthorizedSOAP(t *testing.T) {
	payload := []byte(`
		<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope">
		  <soap:Body>
		    <nfeResultMsg xmlns="http://www.portalfiscal.inf.br/nfe/wsdl/NFeAutorizacao4">
		      <retEnviNFe xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00">
		        <tpAmb>2</tpAmb>
		        <verAplic>TEST-1.0</verAplic>
		        <cStat>104</cStat>
		        <xMotivo>Lote processado</xMotivo>
		        <protNFe versao="4.00">
		          <infProt>
		            <tpAmb>2</tpAmb>
		            <verAplic>TEST-1.0</verAplic>
		            <chNFe>` + testAccessKey + `</chNFe>
		            <dhRecbto>2026-09-30T23:00:00-03:00</dhRecbto>
		            <nProt>131260000000001</nProt>
		            <digVal>ZmFrZWRpZ2VzdA==</digVal>
		            <cStat>100</cStat>
		            <xMotivo>Autorizado o uso da NF-e</xMotivo>
		          </infProt>
		        </protNFe>
		      </retEnviNFe>
		    </nfeResultMsg>
		  </soap:Body>
		</soap:Envelope>`)

	got, err := ParseAuthorizationResponse(payload)
	if err != nil {
		t.Fatalf("ParseAuthorizationResponse: %v", err)
	}
	if got.StatusCode != 104 || !got.Authorized() {
		t.Fatalf("unexpected authorization response: %+v", got)
	}
	if got.Protocol == nil || got.Protocol.AccessKey != testAccessKey || got.Protocol.Protocol != "131260000000001" {
		t.Fatalf("unexpected protocol: %+v", got.Protocol)
	}
}

func TestParseAuthorizationResponseReceipt(t *testing.T) {
	payload := []byte(`
		<retEnviNFe xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00">
		  <tpAmb>2</tpAmb><verAplic>TEST</verAplic><cStat>103</cStat>
		  <xMotivo>Lote recebido com sucesso</xMotivo>
		  <infRec><nRec>310000000000001</nRec><tMed>1</tMed></infRec>
		</retEnviNFe>`)
	got, err := ParseAuthorizationResponse(payload)
	if err != nil {
		t.Fatalf("ParseAuthorizationResponse: %v", err)
	}
	if !got.NeedsReceiptPoll() || got.Receipt.Number != "310000000000001" {
		t.Fatalf("unexpected receipt response: %+v", got)
	}
}

func TestParseServiceStatusResponse(t *testing.T) {
	payload := []byte(`
		<retConsStatServ xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00">
		  <tpAmb>2</tpAmb><verAplic>TEST</verAplic><cStat>107</cStat>
		  <xMotivo>Servico em Operacao</xMotivo><cUF>31</cUF>
		  <dhRecbto>2026-09-30T23:00:00-03:00</dhRecbto><tMed>1</tMed>
		</retConsStatServ>`)
	got, err := ParseServiceStatusResponse(payload)
	if err != nil {
		t.Fatalf("ParseServiceStatusResponse: %v", err)
	}
	if !got.Available() || got.UFCode != "31" {
		t.Fatalf("unexpected service status: %+v", got)
	}
}

func TestParseConsultationResponseAuthorized(t *testing.T) {
	payload := []byte(`
		<retConsSitNFe xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00">
		  <tpAmb>2</tpAmb><verAplic>TEST</verAplic><cStat>100</cStat>
		  <xMotivo>Autorizado o uso da NF-e</xMotivo><chNFe>` + testAccessKey + `</chNFe>
		  <protNFe versao="4.00"><infProt>
		    <chNFe>` + testAccessKey + `</chNFe>
		    <dhRecbto>2026-09-30T23:00:00-03:00</dhRecbto>
		    <nProt>131260000000001</nProt><digVal>ZmFrZWRpZ2VzdA==</digVal>
		    <cStat>100</cStat><xMotivo>Autorizado o uso da NF-e</xMotivo>
		  </infProt></protNFe>
		</retConsSitNFe>`)
	got, err := ParseConsultationResponse(payload)
	if err != nil {
		t.Fatalf("ParseConsultationResponse: %v", err)
	}
	if !got.Authorized() || got.AccessKey != testAccessKey || got.Protocol == nil {
		t.Fatalf("unexpected consultation response: %+v", got)
	}
}
