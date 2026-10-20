package sefaz

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

const (
	NFeNamespace = "http://www.portalfiscal.inf.br/nfe"
	Version400   = "4.00"
)

type Environment string

const (
	EnvironmentProduction   Environment = "production"
	EnvironmentHomologation Environment = "homologation"
)

func tpAmb(env Environment) (string, error) {
	switch env {
	case EnvironmentProduction:
		return "1", nil
	case EnvironmentHomologation:
		return "2", nil
	default:
		return "", fmt.Errorf("invalid SEFAZ environment %q", env)
	}
}

type StatusRequest struct {
	XMLName xml.Name `xml:"consStatServ"`
	Xmlns   string   `xml:"xmlns,attr"`
	Version string   `xml:"versao,attr"`
	TpAmb   string   `xml:"tpAmb"`
	CUF     string   `xml:"cUF"`
	XServ   string   `xml:"xServ"`
}

func BuildStatusRequest(env Environment, uf string) ([]byte, error) {
	ambient, err := tpAmb(env)
	if err != nil {
		return nil, err
	}
	code, ok := fisc.UFCode(uf)
	if !ok {
		return nil, fmt.Errorf("invalid issuer UF %q", uf)
	}
	req := StatusRequest{
		Xmlns: NFeNamespace, Version: Version400, TpAmb: ambient, CUF: code, XServ: "STATUS",
	}
	return marshalDocument(req)
}

type ConsultationRequest struct {
	XMLName xml.Name `xml:"consSitNFe"`
	Xmlns   string   `xml:"xmlns,attr"`
	Version string   `xml:"versao,attr"`
	TpAmb   string   `xml:"tpAmb"`
	XServ   string   `xml:"xServ"`
	ChNFe   string   `xml:"chNFe"`
}

func BuildConsultationRequest(env Environment, accessKey string) ([]byte, error) {
	if err := fisc.ValidateNFCeAccessKey(accessKey); err != nil {
		return nil, fmt.Errorf("invalid access key: %w", err)
	}
	ambient, err := tpAmb(env)
	if err != nil {
		return nil, err
	}
	req := ConsultationRequest{
		Xmlns: NFeNamespace, Version: Version400, TpAmb: ambient, XServ: "CONSULTAR",
		ChNFe: strings.TrimSpace(accessKey),
	}
	return marshalDocument(req)
}

type AuthorizationBatch struct {
	XMLName xml.Name `xml:"enviNFe"`
	Xmlns   string   `xml:"xmlns,attr"`
	Version string   `xml:"versao,attr"`
	IDLot   string   `xml:"idLote"`
	IndSinc int      `xml:"indSinc"`
	NFeXML  []byte   `xml:",innerxml"`
}

func BuildAuthorizationBatch(idLot string, signedNFe []byte) ([]byte, error) {
	idLot = strings.TrimSpace(idLot)
	if len(idLot) < 1 || len(idLot) > 15 || !allDigits(idLot) {
		return nil, fmt.Errorf("idLote must contain 1 to 15 digits")
	}
	signedNFe = stripXMLHeader(signedNFe)
	if err := requireXMLDocumentRoot(signedNFe, "NFe"); err != nil {
		return nil, fmt.Errorf("signed NFC-e: %w", err)
	}
	req := AuthorizationBatch{
		Xmlns: NFeNamespace, Version: Version400, IDLot: idLot, IndSinc: 1, NFeXML: signedNFe,
	}
	return marshalDocument(req)
}

type Protocol struct {
	AccessKey   string
	ReceivedAt  string
	Protocol    string
	DigestValue string
	RawXML      []byte
	StatusCode  int
	Reason      string
}

type Receipt struct {
	Number      string
	AverageTime string
}

type AuthorizationResponse struct {
	Environment string
	AppVersion  string
	StatusCode  int
	Reason      string
	Receipt     *Receipt
	Protocol    *Protocol
}

func (r AuthorizationResponse) Authorized() bool {
	return r.Protocol != nil && r.Protocol.StatusCode == 100
}

func (r AuthorizationResponse) NeedsReceiptPoll() bool {
	return r.StatusCode == 103 && r.Receipt != nil && strings.TrimSpace(r.Receipt.Number) != ""
}

type authorizationXML struct {
	TpAmb    string `xml:"tpAmb"`
	VerAplic string `xml:"verAplic"`
	CStat    string `xml:"cStat"`
	XMotivo  string `xml:"xMotivo"`
	InfRec   *struct {
		NRec string `xml:"nRec"`
		TMed string `xml:"tMed"`
	} `xml:"infRec"`
	ProtNFe *struct {
		InfProt struct {
			ChNFe    string `xml:"chNFe"`
			DhRecbto string `xml:"dhRecbto"`
			NProt    string `xml:"nProt"`
			DigVal   string `xml:"digVal"`
			CStat    string `xml:"cStat"`
			XMotivo  string `xml:"xMotivo"`
		} `xml:"infProt"`
	} `xml:"protNFe"`
}

func ParseAuthorizationResponse(content []byte) (AuthorizationResponse, error) {
	var raw authorizationXML
	if err := decodeElementByLocalName(content, "retEnviNFe", &raw); err != nil {
		return AuthorizationResponse{}, err
	}
	code, err := parseStatusCode(raw.CStat)
	if err != nil {
		return AuthorizationResponse{}, fmt.Errorf("authorization cStat: %w", err)
	}
	out := AuthorizationResponse{
		Environment: strings.TrimSpace(raw.TpAmb),
		AppVersion:  strings.TrimSpace(raw.VerAplic),
		StatusCode:  code,
		Reason:      strings.TrimSpace(raw.XMotivo),
	}
	if raw.InfRec != nil {
		out.Receipt = &Receipt{
			Number: strings.TrimSpace(raw.InfRec.NRec), AverageTime: strings.TrimSpace(raw.InfRec.TMed),
		}
	}
	if raw.ProtNFe != nil {
		protoXML, err := ExtractProtocolXML(content)
		if err != nil {
			return AuthorizationResponse{}, err
		}
		protoCode, err := parseStatusCode(raw.ProtNFe.InfProt.CStat)
		if err != nil {
			return AuthorizationResponse{}, fmt.Errorf("protocol cStat: %w", err)
		}
		accessKey := strings.TrimSpace(raw.ProtNFe.InfProt.ChNFe)
		if accessKey != "" {
			if err := fisc.ValidateNFCeAccessKey(accessKey); err != nil {
				return AuthorizationResponse{}, fmt.Errorf("protocol access key: %w", err)
			}
		}
		out.Protocol = &Protocol{
			AccessKey: accessKey, ReceivedAt: strings.TrimSpace(raw.ProtNFe.InfProt.DhRecbto),
			Protocol:    strings.TrimSpace(raw.ProtNFe.InfProt.NProt),
			DigestValue: strings.TrimSpace(raw.ProtNFe.InfProt.DigVal),
			RawXML:      protoXML,
			StatusCode:  protoCode, Reason: strings.TrimSpace(raw.ProtNFe.InfProt.XMotivo),
		}
	}
	return out, nil
}

type ServiceStatusResponse struct {
	Environment string
	AppVersion  string
	StatusCode  int
	Reason      string
	UFCode      string
	ReceivedAt  string
	AverageTime string
	Observation string
}

func (r ServiceStatusResponse) Available() bool {
	return r.StatusCode == 107
}

type statusXML struct {
	TpAmb    string `xml:"tpAmb"`
	VerAplic string `xml:"verAplic"`
	CStat    string `xml:"cStat"`
	XMotivo  string `xml:"xMotivo"`
	CUF      string `xml:"cUF"`
	DhRecbto string `xml:"dhRecbto"`
	TMed     string `xml:"tMed"`
	XObs     string `xml:"xObs"`
}

func ParseServiceStatusResponse(content []byte) (ServiceStatusResponse, error) {
	var raw statusXML
	if err := decodeElementByLocalName(content, "retConsStatServ", &raw); err != nil {
		return ServiceStatusResponse{}, err
	}
	code, err := parseStatusCode(raw.CStat)
	if err != nil {
		return ServiceStatusResponse{}, err
	}
	return ServiceStatusResponse{
		Environment: strings.TrimSpace(raw.TpAmb), AppVersion: strings.TrimSpace(raw.VerAplic),
		StatusCode: code, Reason: strings.TrimSpace(raw.XMotivo), UFCode: strings.TrimSpace(raw.CUF),
		ReceivedAt: strings.TrimSpace(raw.DhRecbto), AverageTime: strings.TrimSpace(raw.TMed),
		Observation: strings.TrimSpace(raw.XObs),
	}, nil
}

type ConsultationResponse struct {
	Environment string
	AppVersion  string
	StatusCode  int
	Reason      string
	AccessKey   string
	Protocol    *Protocol
}

func (r ConsultationResponse) Authorized() bool {
	return r.Protocol != nil && r.Protocol.StatusCode == 100
}

type consultationXML struct {
	TpAmb    string `xml:"tpAmb"`
	VerAplic string `xml:"verAplic"`
	CStat    string `xml:"cStat"`
	XMotivo  string `xml:"xMotivo"`
	ChNFe    string `xml:"chNFe"`
	ProtNFe  *struct {
		InfProt struct {
			ChNFe    string `xml:"chNFe"`
			DhRecbto string `xml:"dhRecbto"`
			NProt    string `xml:"nProt"`
			DigVal   string `xml:"digVal"`
			CStat    string `xml:"cStat"`
			XMotivo  string `xml:"xMotivo"`
		} `xml:"infProt"`
	} `xml:"protNFe"`
}

func ParseConsultationResponse(content []byte) (ConsultationResponse, error) {
	var raw consultationXML
	if err := decodeElementByLocalName(content, "retConsSitNFe", &raw); err != nil {
		return ConsultationResponse{}, err
	}
	code, err := parseStatusCode(raw.CStat)
	if err != nil {
		return ConsultationResponse{}, err
	}
	out := ConsultationResponse{
		Environment: strings.TrimSpace(raw.TpAmb), AppVersion: strings.TrimSpace(raw.VerAplic),
		StatusCode: code, Reason: strings.TrimSpace(raw.XMotivo), AccessKey: strings.TrimSpace(raw.ChNFe),
	}
	if out.AccessKey != "" {
		if err := fisc.ValidateNFCeAccessKey(out.AccessKey); err != nil {
			return ConsultationResponse{}, fmt.Errorf("consultation access key: %w", err)
		}
	}
	if raw.ProtNFe != nil {
		protoXML, err := ExtractProtocolXML(content)
		if err != nil {
			return ConsultationResponse{}, err
		}
		protoCode, err := parseStatusCode(raw.ProtNFe.InfProt.CStat)
		if err != nil {
			return ConsultationResponse{}, err
		}
		out.Protocol = &Protocol{
			AccessKey:   strings.TrimSpace(raw.ProtNFe.InfProt.ChNFe),
			ReceivedAt:  strings.TrimSpace(raw.ProtNFe.InfProt.DhRecbto),
			Protocol:    strings.TrimSpace(raw.ProtNFe.InfProt.NProt),
			DigestValue: strings.TrimSpace(raw.ProtNFe.InfProt.DigVal),
			RawXML:      protoXML,
			StatusCode:  protoCode, Reason: strings.TrimSpace(raw.ProtNFe.InfProt.XMotivo),
		}
	}
	return out, nil
}

func marshalDocument(value any) ([]byte, error) {
	content, err := xml.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), content...), nil
}

func requireXMLDocumentRoot(content []byte, want string) error {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	foundRoot := false
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if !foundRoot || depth != 0 {
				return fmt.Errorf("invalid XML document")
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("invalid XML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			if !foundRoot {
				foundRoot = true
				if value.Name.Local != want {
					return fmt.Errorf("root element=%s, want %s", value.Name.Local, want)
				}
			}
			depth++
		case xml.EndElement:
			depth--
			if depth < 0 {
				return fmt.Errorf("invalid XML nesting")
			}
		}
	}
}

func decodeElementByLocalName(content []byte, local string, out any) error {
	decoder := xml.NewDecoder(bytes.NewReader(content))
	for {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("%s not found: %w", local, err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != local {
			continue
		}
		if err := decoder.DecodeElement(out, &start); err != nil {
			return fmt.Errorf("decode %s: %w", local, err)
		}
		return nil
	}
}

func parseStatusCode(value string) (int, error) {
	value = strings.TrimSpace(value)
	code, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid status code %q", value)
	}
	return code, nil
}

func allDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}
