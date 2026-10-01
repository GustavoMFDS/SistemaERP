package sefaz

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGatewayStatusAuthorizeConsult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		text := string(body)
		w.Header().Set("Content-Type", "application/soap+xml")
		switch {
		case strings.Contains(text, "<consStatServ"):
			_, _ = w.Write([]byte(`<retConsStatServ xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00"><tpAmb>2</tpAmb><verAplic>TEST</verAplic><cStat>107</cStat><xMotivo>Servico em Operacao</xMotivo><cUF>31</cUF><dhRecbto>2026-09-30T23:00:00-03:00</dhRecbto><tMed>1</tMed></retConsStatServ>`))
		case strings.Contains(text, "<enviNFe"):
			_, _ = w.Write([]byte(`<retEnviNFe xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00"><tpAmb>2</tpAmb><verAplic>TEST</verAplic><cStat>104</cStat><xMotivo>Lote processado</xMotivo><protNFe versao="4.00"><infProt><chNFe>` + testAccessKey + `</chNFe><dhRecbto>2026-09-30T23:00:00-03:00</dhRecbto><nProt>131260000000001</nProt><digVal>ZmFrZQ==</digVal><cStat>100</cStat><xMotivo>Autorizado o uso da NF-e</xMotivo></infProt></protNFe></retEnviNFe>`))
		case strings.Contains(text, "<consSitNFe"):
			_, _ = w.Write([]byte(`<retConsSitNFe xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00"><tpAmb>2</tpAmb><verAplic>TEST</verAplic><cStat>100</cStat><xMotivo>Autorizado o uso da NF-e</xMotivo><chNFe>` + testAccessKey + `</chNFe><protNFe versao="4.00"><infProt><chNFe>` + testAccessKey + `</chNFe><dhRecbto>2026-09-30T23:00:00-03:00</dhRecbto><nProt>131260000000001</nProt><digVal>ZmFrZQ==</digVal><cStat>100</cStat><xMotivo>Autorizado o uso da NF-e</xMotivo></infProt></protNFe></retConsSitNFe>`))
		default:
			http.Error(w, "unexpected payload", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	endpoints := ServiceEndpoints{
		Status: Endpoint{URL: server.URL, WSDLNamespace: WSDLStatusService},
		Authorization: Endpoint{URL: server.URL, WSDLNamespace: WSDLAuthorizationService},
		Consultation: Endpoint{URL: server.URL, WSDLNamespace: WSDLConsultationService},
	}
	gateway, err := NewGateway(NewSOAPClient(nil, 2*time.Second), endpoints)
	if err != nil {
		t.Fatalf("NewGateway: %v", err)
	}

	status, err := gateway.Status(context.Background(), EnvironmentHomologation, "MG")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.Available() {
		t.Fatalf("status unavailable: %+v", status)
	}

	signed := []byte(`<NFe xmlns="http://www.portalfiscal.inf.br/nfe"><infNFe Id="NFe` + testAccessKey + `" versao="4.00"></infNFe><Signature xmlns="http://www.w3.org/2000/09/xmldsig#"></Signature></NFe>`)
	auth, err := gateway.Authorize(context.Background(), "1", testAccessKey, signed)
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if !auth.Authorized() || auth.Protocol == nil || auth.Protocol.Protocol == "" {
		t.Fatalf("authorization failed: %+v", auth)
	}

	consulted, err := gateway.Consult(context.Background(), EnvironmentHomologation, testAccessKey)
	if err != nil {
		t.Fatalf("Consult: %v", err)
	}
	if !consulted.Authorized() {
		t.Fatalf("consultation not authorized: %+v", consulted)
	}
}

func TestGatewayRejectsMismatchedProtocolAccessKey(t *testing.T) {
	const otherKey = "31260912ABC34501DE35650010000000431123456789"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<retEnviNFe xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00"><tpAmb>2</tpAmb><verAplic>TEST</verAplic><cStat>104</cStat><xMotivo>Lote processado</xMotivo><protNFe versao="4.00"><infProt><chNFe>` + otherKey + `</chNFe><dhRecbto>2026-09-30T23:00:00-03:00</dhRecbto><nProt>131260000000001</nProt><digVal>ZmFrZQ==</digVal><cStat>100</cStat><xMotivo>Autorizado o uso da NF-e</xMotivo></infProt></protNFe></retEnviNFe>`))
	}))
	defer server.Close()

	gateway, err := NewGateway(NewSOAPClient(nil, 2*time.Second), ServiceEndpoints{
		Status: Endpoint{URL: server.URL, WSDLNamespace: WSDLStatusService},
		Authorization: Endpoint{URL: server.URL, WSDLNamespace: WSDLAuthorizationService},
		Consultation: Endpoint{URL: server.URL, WSDLNamespace: WSDLConsultationService},
	})
	if err != nil {
		t.Fatal(err)
	}
	signed := []byte(`<NFe xmlns="http://www.portalfiscal.inf.br/nfe"><infNFe Id="NFe` + testAccessKey + `" versao="4.00"></infNFe></NFe>`)
	if _, err := gateway.Authorize(context.Background(), "1", testAccessKey, signed); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected access-key mismatch error, got %v", err)
	}
}
