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

const authorizationWSDL = SEFAZWSDLNamespacePrefix + "NFeAutorizacao4"

func TestBuildSOAP12Envelope(t *testing.T) {
	payload := []byte(xmlHeaderForTest() + `<consStatServ xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00"><tpAmb>2</tpAmb></consStatServ>`)
	content, err := BuildSOAP12Envelope(authorizationWSDL, payload)
	if err != nil {
		t.Fatalf("BuildSOAP12Envelope: %v", err)
	}
	xml := string(content)
	if strings.Count(xml, "<?xml") != 1 {
		t.Fatalf("nested XML declaration found: %s", xml)
	}
	for _, want := range []string{
		SOAP12Namespace,
		authorizationWSDL,
		"<consStatServ",
		"<tpAmb>2</tpAmb>",
	} {
		if !strings.Contains(xml, want) {
			t.Fatalf("SOAP envelope missing %q: %s", want, xml)
		}
	}
}

func TestEndpointValidation(t *testing.T) {
	valid := []Endpoint{
		{URL: "https://example.invalid/NFeAutorizacao4", WSDLNamespace: authorizationWSDL},
		{URL: "http://127.0.0.1:8080/test", WSDLNamespace: authorizationWSDL},
		{URL: "http://localhost:8080/test", WSDLNamespace: authorizationWSDL},
	}
	for _, endpoint := range valid {
		if err := endpoint.Validate(); err != nil {
			t.Fatalf("valid endpoint rejected: %+v err=%v", endpoint, err)
		}
	}

	invalid := []Endpoint{
		{URL: "http://sefaz.example.test/NFeAutorizacao4", WSDLNamespace: authorizationWSDL},
		{URL: "https://user:pass@example.test/ws", WSDLNamespace: authorizationWSDL},
		{URL: "https://example.test/ws#fragment", WSDLNamespace: authorizationWSDL},
		{URL: "https://example.test/ws", WSDLNamespace: "https://attacker.invalid/wsdl"},
	}
	for _, endpoint := range invalid {
		if err := endpoint.Validate(); err == nil {
			t.Fatalf("invalid endpoint accepted: %+v", endpoint)
		}
	}
}

func TestSOAPClientPost(t *testing.T) {
	var gotContentType string
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/soap+xml")
		_, _ = w.Write([]byte(`<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope"><soap:Body><ok/></soap:Body></soap:Envelope>`))
	}))
	defer server.Close()

	client := NewSOAPClient(nil, 2*time.Second)
	payload := []byte(`<consStatServ xmlns="http://www.portalfiscal.inf.br/nfe" versao="4.00"/>`)
	response, err := client.Post(context.Background(), Endpoint{
		URL: server.URL, WSDLNamespace: authorizationWSDL,
	}, payload)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if !strings.HasPrefix(gotContentType, "application/soap+xml") {
		t.Fatalf("Content-Type=%q", gotContentType)
	}
	if !strings.Contains(gotBody, authorizationWSDL) || !strings.Contains(gotBody, "<consStatServ") {
		t.Fatalf("unexpected SOAP request body: %s", gotBody)
	}
	if !strings.Contains(string(response), "<ok") {
		t.Fatalf("unexpected response: %s", response)
	}
}

func TestSOAPClientDoesNotFollowRedirects(t *testing.T) {
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits++
		_, _ = w.Write([]byte("<ok/>"))
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	client := NewSOAPClient(nil, 2*time.Second)
	_, err := client.Post(context.Background(), Endpoint{
		URL: redirect.URL, WSDLNamespace: authorizationWSDL,
	}, []byte("<x/>"))
	if err == nil {
		t.Fatal("expected redirect response to fail")
	}
	if targetHits != 0 {
		t.Fatalf("client followed redirect %d times", targetHits)
	}
}

func TestSOAPClientResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<root>" + strings.Repeat("x", 128) + "</root>"))
	}))
	defer server.Close()

	client := NewSOAPClient(nil, 2*time.Second)
	client.SetMaxResponseBytes(32)
	_, err := client.Post(context.Background(), Endpoint{
		URL: server.URL, WSDLNamespace: authorizationWSDL,
	}, []byte("<x/>"))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected response-size error, got %v", err)
	}
}

func xmlHeaderForTest() string {
	return "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n"
}
