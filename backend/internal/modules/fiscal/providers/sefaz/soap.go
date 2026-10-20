package sefaz

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	SOAP12Namespace          = "http://www.w3.org/2003/05/soap-envelope"
	SEFAZWSDLNamespacePrefix = "http://www.portalfiscal.inf.br/nfe/wsdl/"
	DefaultMaxResponseBytes  = 4 << 20
)

type Endpoint struct {
	URL           string
	WSDLNamespace string
}

func (e Endpoint) Validate() error {
	parsed, err := url.Parse(strings.TrimSpace(e.URL))
	if err != nil {
		return fmt.Errorf("invalid SEFAZ endpoint URL: %w", err)
	}
	if parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("invalid SEFAZ endpoint URL")
	}
	if parsed.Scheme != "https" {
		host := parsed.Hostname()
		if parsed.Scheme != "http" || !isLoopbackHost(host) {
			return fmt.Errorf("SEFAZ endpoint must use HTTPS")
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(e.WSDLNamespace), SEFAZWSDLNamespacePrefix) {
		return fmt.Errorf("invalid SEFAZ WSDL namespace")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type soapEnvelope struct {
	XMLName xml.Name `xml:"http://www.w3.org/2003/05/soap-envelope Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	Message soapMessage `xml:"nfeDadosMsg"`
}

type soapMessage struct {
	Xmlns   string `xml:"xmlns,attr"`
	Payload []byte `xml:",innerxml"`
}

func BuildSOAP12Envelope(wsdlNamespace string, payload []byte) ([]byte, error) {
	wsdlNamespace = strings.TrimSpace(wsdlNamespace)
	if !strings.HasPrefix(wsdlNamespace, SEFAZWSDLNamespacePrefix) {
		return nil, fmt.Errorf("invalid SEFAZ WSDL namespace")
	}
	payload = stripXMLHeader(payload)
	if err := requireAnyXMLRoot(payload); err != nil {
		return nil, err
	}
	envelope := soapEnvelope{
		Body: soapBody{
			Message: soapMessage{Xmlns: wsdlNamespace, Payload: payload},
		},
	}
	content, err := xml.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), content...), nil
}

func stripXMLHeader(content []byte) []byte {
	trimmed := bytes.TrimSpace(content)
	header := []byte(xml.Header)
	if bytes.HasPrefix(trimmed, header) {
		return bytes.TrimSpace(trimmed[len(header):])
	}
	return trimmed
}

func requireAnyXMLRoot(content []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	for {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("invalid XML payload: %w", err)
		}
		if _, ok := token.(xml.StartElement); ok {
			return nil
		}
	}
}

type SOAPClient struct {
	client           *http.Client
	maxResponseBytes int64
}

func NewSOAPClient(transport http.RoundTripper, timeout time.Duration) *SOAPClient {
	if transport == nil {
		transport = http.DefaultTransport
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &SOAPClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		maxResponseBytes: DefaultMaxResponseBytes,
	}
}

func (c *SOAPClient) SetMaxResponseBytes(value int64) {
	if value > 0 {
		c.maxResponseBytes = value
	}
}

func (c *SOAPClient) Post(ctx context.Context, endpoint Endpoint, payload []byte) ([]byte, error) {
	if err := endpoint.Validate(); err != nil {
		return nil, err
	}
	envelope, err := BuildSOAP12Envelope(endpoint.WSDLNamespace, payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(envelope))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("Accept", "application/soap+xml, application/xml, text/xml")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SEFAZ SOAP request: %w", err)
	}
	defer resp.Body.Close()

	limit := c.maxResponseBytes
	if limit <= 0 {
		limit = DefaultMaxResponseBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read SEFAZ SOAP response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("SEFAZ SOAP response exceeds %d bytes", limit)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("SEFAZ SOAP HTTP status %d", resp.StatusCode)
	}
	if err := requireAnyXMLRoot(body); err != nil {
		return nil, fmt.Errorf("SEFAZ SOAP response: %w", err)
	}
	return body, nil
}
