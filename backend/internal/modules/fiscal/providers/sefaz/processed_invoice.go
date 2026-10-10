package sefaz

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

// ExtractProtocolXML isolates exactly the protNFe element returned by a
// trusted SEFAZ response. We preserve its bytes, never recreate an invented
// authorization protocol from status fields supplied by the application.
func ExtractProtocolXML(payload []byte) ([]byte, error) {
	if len(payload) == 0 || len(payload) > DefaultMaxResponseBytes {
		return nil, fmt.Errorf("SEFAZ protocol response has invalid size")
	}
	dec := xml.NewDecoder(bytes.NewReader(payload))
	var startOffset int64 = -1
	depth := 0
	found := false
	var result []byte
	for {
		before := dec.InputOffset()
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("parse SEFAZ protocol response: %w", err)
		}
		switch v := tok.(type) {
		case xml.Directive:
			return nil, fmt.Errorf("DTD/directive not allowed in SEFAZ response")
		case xml.StartElement:
			if startOffset >= 0 {
				depth++
			} else if v.Name.Local == "protNFe" && v.Name.Space == NFeNamespace {
				if found {
					return nil, fmt.Errorf("multiple SEFAZ protocols")
				}
				found = true
				startOffset = before
				depth = 1
			}
		case xml.EndElement:
			if startOffset >= 0 {
				depth--
				if depth == 0 {
					result = append([]byte(nil), payload[startOffset:dec.InputOffset()]...)
					startOffset = -1
				}
			}
		}
	}
	if !found || startOffset >= 0 || len(result) == 0 {
		return nil, fmt.Errorf("SEFAZ protNFe element not found")
	}
	return result, nil
}

type signedDocumentForProc struct {
	XMLName xml.Name `xml:"http://www.portalfiscal.inf.br/nfe NFe"`
	InfNFe  struct {
		ID      string `xml:"Id,attr"`
		Version string `xml:"versao,attr"`
		Ide     struct {
			Model       string `xml:"mod"`
			Environment string `xml:"tpAmb"`
		} `xml:"ide"`
	} `xml:"infNFe"`
	Signature struct {
		SignedInfo struct {
			Reference struct {
				URI    string `xml:"URI,attr"`
				Digest string `xml:"DigestValue"`
			} `xml:"Reference"`
		} `xml:"SignedInfo"`
	} `xml:"http://www.w3.org/2000/09/xmldsig# Signature"`
}
type sefazProcessedProtocol struct {
	XMLName xml.Name `xml:"http://www.portalfiscal.inf.br/nfe protNFe"`
	Version string   `xml:"versao,attr"`
	InfProt struct {
		Environment string `xml:"tpAmb"`
		Application string `xml:"verAplic"`
		Key         string `xml:"chNFe"`
		ReceivedAt  string `xml:"dhRecbto"`
		Protocol    string `xml:"nProt"`
		Digest      string `xml:"digVal"`
		Status      string `xml:"cStat"`
		Reason      string `xml:"xMotivo"`
	} `xml:"infProt"`
}

// BuildAuthorizedNFeProc accepts ONLY the originally signed NFC-e XML and
// the original protNFe element captured from a SEFAZ response. The invoice
// must already have passed through cryptographic XMLDSig verification and
// official schema validation when it was signed. This method checks that
// the response truly belongs to the SAME signed invoice; it neither
// synthesizes a protocol nor asserts trust in a fake external response.
func BuildAuthorizedNFeProc(signedXML, protocolXML []byte, accessKey, expectedProtocol string, authorizedAt time.Time) ([]byte, error) {
	if err := fisc.ValidateNFCeAccessKey(accessKey); err != nil {
		return nil, fmt.Errorf("invalid processed invoice access key: %w", err)
	}
	if len(signedXML) == 0 || len(protocolXML) == 0 || len(signedXML) > 2<<20 || len(protocolXML) > 128<<10 {
		return nil, fmt.Errorf("NFe/protNFe content missing or oversized")
	}
	if authorizedAt.IsZero() || strings.TrimSpace(expectedProtocol) == "" {
		return nil, fmt.Errorf("authorized timestamp and genuine protocol are required")
	}
	var signed signedDocumentForProc
	if err := xml.Unmarshal(signedXML, &signed); err != nil {
		return nil, fmt.Errorf("parse signed NFe: %w", err)
	}
	if signed.XMLName.Local != "NFe" || signed.XMLName.Space != NFeNamespace ||
		signed.InfNFe.ID != "NFe"+accessKey || signed.InfNFe.Version != Version400 ||
		signed.InfNFe.Ide.Model != "65" {
		return nil, fmt.Errorf("signed NFC-e identity/model mismatch")
	}
	if signed.InfNFe.Ide.Environment != "1" && signed.InfNFe.Ide.Environment != "2" {
		return nil, fmt.Errorf("signed NFC-e environment missing")
	}
	if signed.Signature.SignedInfo.Reference.URI != "#NFe"+accessKey {
		return nil, fmt.Errorf("signed NFC-e Signature Reference is missing or wrong")
	}
	digest := strings.TrimSpace(signed.Signature.SignedInfo.Reference.Digest)
	rawDigest, err := base64.StdEncoding.DecodeString(digest)
	if err != nil || len(rawDigest) != 20 {
		return nil, fmt.Errorf("signed NFC-e digest invalid")
	}
	// Enforce namespace without changing original protocol bytes.
	// A protocol relying on an inherited default xmlns is legal when nested
	// under nfeProc. For parsing outside that context, add the same wrapper.
	wrapped := append([]byte(`<nfeProc xmlns="`+NFeNamespace+`" versao="4.00">`), protocolXML...)
	wrapped = append(wrapped, []byte("</nfeProc>")...)
	var container struct {
		Protocol sefazProcessedProtocol `xml:"protNFe"`
	}
	if err := xml.Unmarshal(wrapped, &container); err != nil {
		return nil, fmt.Errorf("parse original SEFAZ protocol: %w", err)
	}
	p := container.Protocol
	if p.XMLName.Local != "protNFe" || p.XMLName.Space != NFeNamespace ||
		p.Version != Version400 || p.InfProt.Key != accessKey ||
		p.InfProt.Environment != signed.InfNFe.Ide.Environment ||
		p.InfProt.Protocol != expectedProtocol || p.InfProt.Status != "100" ||
		strings.TrimSpace(p.InfProt.Application) == "" ||
		strings.TrimSpace(p.InfProt.Reason) == "" ||
		strings.TrimSpace(p.InfProt.Digest) != digest {
		return nil, fmt.Errorf("SEFAZ protocol cannot be matched to the signed NFC-e")
	}
	receivedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(p.InfProt.ReceivedAt))
	if err != nil || !receivedAt.Equal(authorizedAt) {
		return nil, fmt.Errorf("SEFAZ protocol timestamp does not match authorization")
	}
	// Preserve the signed XML and the genuine protNFe fragment, byte for byte.
	// Only the wrapping nfeProc document and optional XML header are new.
	signedXML = stripXMLHeader(signedXML)
	if err := requireXMLDocumentRoot(signedXML, "NFe"); err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<nfeProc xmlns="` + NFeNamespace + `" versao="4.00">`)
	b.Write(signedXML)
	b.Write(protocolXML)
	b.WriteString("</nfeProc>")
	var structure struct {
		XMLName  xml.Name               `xml:"http://www.portalfiscal.inf.br/nfe nfeProc"`
		NFe      signedDocumentForProc  `xml:"NFe"`
		Protocol sefazProcessedProtocol `xml:"protNFe"`
	}
	if err := xml.Unmarshal(b.Bytes(), &structure); err != nil {
		return nil, fmt.Errorf("validate processed XML envelope: %w", err)
	}
	return b.Bytes(), nil
}

// ProcessedDocumentBuilder is the production adapter for the application
// port; the real SEFAZ protocol must be provided by its remote authorizer.
type ProcessedDocumentBuilder struct{}

func NewProcessedDocumentBuilder() *ProcessedDocumentBuilder { return &ProcessedDocumentBuilder{} }

func (b *ProcessedDocumentBuilder) Build(
	signedXML, protocolXML []byte, accessKey, protocol string, authorizedAt time.Time,
) ([]byte, error) {
	return BuildAuthorizedNFeProc(signedXML, protocolXML, accessKey, protocol, authorizedAt)
}
