package sefaz

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

const (
	CancellationEventType    = "110111"
	CancellationEventVersion = "1.00"
)

type CancellationEventInput struct {
	Environment           Environment
	IssuerUF              string
	IssuerCNPJ            string
	AccessKey             string
	AuthorizationProtocol string
	EventTime             time.Time
	Sequence              int
	Justification         string
}

type eventEnvelopeXML struct {
	XMLName xml.Name   `xml:"envEvento"`
	Xmlns   string     `xml:"xmlns,attr"`
	Version string     `xml:"versao,attr"`
	IDLot   string     `xml:"idLote"`
	Event   eventXML   `xml:"evento"`
}

type eventXML struct {
	Version   string       `xml:"versao,attr"`
	InfEvento infEventoXML `xml:"infEvento"`
}

type infEventoXML struct {
	ID         string         `xml:"Id,attr"`
	COrgao     string         `xml:"cOrgao"`
	TpAmb      string         `xml:"tpAmb"`
	CNPJ       string         `xml:"CNPJ"`
	ChNFe      string         `xml:"chNFe"`
	DhEvento   string         `xml:"dhEvento"`
	TpEvento   string         `xml:"tpEvento"`
	NSeqEvento int            `xml:"nSeqEvento"`
	VerEvento  string         `xml:"verEvento"`
	DetEvento  detEventoXML   `xml:"detEvento"`
}

type detEventoXML struct {
	Version    string `xml:"versao,attr"`
	DescEvento string `xml:"descEvento"`
	NProt      string `xml:"nProt"`
	XJust      string `xml:"xJust"`
}

func BuildUnsignedCancellationEvent(input CancellationEventInput) ([]byte, string, error) {
	ambient, err := tpAmb(input.Environment)
	if err != nil {
		return nil, "", err
	}
	cUF, ok := fisc.UFCode(input.IssuerUF)
	if !ok {
		return nil, "", fmt.Errorf("invalid issuer UF")
	}
	if err := fisc.ValidateCNPJ(input.IssuerCNPJ); err != nil {
		return nil, "", fmt.Errorf("issuer CNPJ: %w", err)
	}
	if err := fisc.ValidateNFCeAccessKey(input.AccessKey); err != nil {
		return nil, "", fmt.Errorf("access key: %w", err)
	}
	protocol := strings.TrimSpace(input.AuthorizationProtocol)
	if len(protocol) != 15 || !allDigits(protocol) {
		return nil, "", fmt.Errorf("authorization protocol must contain 15 digits")
	}
	if input.EventTime.IsZero() {
		return nil, "", fmt.Errorf("event time is required")
	}
	if input.Sequence < 1 || input.Sequence > 99 {
		return nil, "", fmt.Errorf("event sequence must be between 1 and 99")
	}
	justification := strings.TrimSpace(input.Justification)
	if len([]rune(justification)) < 15 || len([]rune(justification)) > 255 {
		return nil, "", fmt.Errorf("cancellation justification must contain 15 to 255 characters")
	}

	normalizedCNPJ := normalizedCNPJ(input.IssuerCNPJ)
	eventID := fmt.Sprintf(
		"ID%s%s%02d",
		CancellationEventType,
		strings.TrimSpace(input.AccessKey),
		input.Sequence,
	)
	event := eventXML{
		Version: CancellationEventVersion,
		InfEvento: infEventoXML{
			ID:         eventID,
			COrgao:     cUF,
			TpAmb:      ambient,
			CNPJ:       normalizedCNPJ,
			ChNFe:      strings.TrimSpace(input.AccessKey),
			DhEvento:   input.EventTime.Format(time.RFC3339),
			TpEvento:   CancellationEventType,
			NSeqEvento: input.Sequence,
			VerEvento:  CancellationEventVersion,
			DetEvento: detEventoXML{
				Version:    CancellationEventVersion,
				DescEvento: "Cancelamento",
				NProt:      protocol,
				XJust:      justification,
			},
		},
	}
	payload, err := xml.Marshal(event)
	if err != nil {
		return nil, "", err
	}
	return append([]byte(xml.Header), payload...), eventID, nil
}

func BuildCancellationEventBatch(idLot string, signedEvent []byte) ([]byte, error) {
	idLot = strings.TrimSpace(idLot)
	if idLot == "" || len(idLot) > 15 || !allDigits(idLot) {
		return nil, fmt.Errorf("idLote must contain 1 to 15 digits")
	}
	signedEvent = stripXMLHeader(signedEvent)
	if err := requireXMLDocumentRoot(signedEvent, "evento"); err != nil {
		return nil, fmt.Errorf("signed cancellation event: %w", err)
	}

	type batch struct {
		XMLName xml.Name `xml:"envEvento"`
		Xmlns   string   `xml:"xmlns,attr"`
		Version string   `xml:"versao,attr"`
		IDLot   string   `xml:"idLote"`
		Event   []byte   `xml:",innerxml"`
	}
	return marshalDocument(batch{
		Xmlns: NFeNamespace,
		Version: CancellationEventVersion,
		IDLot: idLot,
		Event: signedEvent,
	})
}

type EventResponse struct {
	BatchStatusCode int
	BatchReason     string
	Environment     string
	AccessKey       string
	EventType       string
	Sequence        int
	StatusCode      int
	Reason          string
	EventName       string
	RegisteredAt    string
	Protocol        string
}

func (r EventResponse) CancellationRegistered() bool {
	return r.EventType == CancellationEventType && r.StatusCode == 135 &&
		strings.TrimSpace(r.Protocol) != ""
}

type retEnvEventoXML struct {
	TpAmb   string `xml:"tpAmb"`
	CStat   string `xml:"cStat"`
	XMotivo string `xml:"xMotivo"`
	RetEvento *struct {
		InfEvento struct {
			TpAmb      string `xml:"tpAmb"`
			VerAplic    string `xml:"verAplic"`
			COrgao      string `xml:"cOrgao"`
			CStat       string `xml:"cStat"`
			XMotivo     string `xml:"xMotivo"`
			ChNFe       string `xml:"chNFe"`
			TpEvento    string `xml:"tpEvento"`
			XEvento     string `xml:"xEvento"`
			NSeqEvento  string `xml:"nSeqEvento"`
			DhRegEvento string `xml:"dhRegEvento"`
			NProt       string `xml:"nProt"`
		} `xml:"infEvento"`
	} `xml:"retEvento"`
}

func ParseEventResponse(content []byte) (EventResponse, error) {
	var raw retEnvEventoXML
	if err := decodeElementByLocalName(content, "retEnvEvento", &raw); err != nil {
		return EventResponse{}, err
	}
	batchCode, err := parseStatusCode(raw.CStat)
	if err != nil {
		return EventResponse{}, fmt.Errorf("event batch cStat: %w", err)
	}
	out := EventResponse{
		BatchStatusCode: batchCode,
		BatchReason: strings.TrimSpace(raw.XMotivo),
		Environment: strings.TrimSpace(raw.TpAmb),
	}
	if raw.RetEvento == nil {
		return out, nil
	}
	itemCode, err := parseStatusCode(raw.RetEvento.InfEvento.CStat)
	if err != nil {
		return EventResponse{}, fmt.Errorf("event cStat: %w", err)
	}
	sequence := 0
	if value := strings.TrimSpace(raw.RetEvento.InfEvento.NSeqEvento); value != "" {
		sequence, err = strconv.Atoi(value)
		if err != nil {
			return EventResponse{}, fmt.Errorf("event sequence: %w", err)
		}
	}
	accessKey := strings.TrimSpace(raw.RetEvento.InfEvento.ChNFe)
	if accessKey != "" {
		if err := fisc.ValidateNFCeAccessKey(accessKey); err != nil {
			return EventResponse{}, fmt.Errorf("event response access key: %w", err)
		}
	}
	out.Environment = strings.TrimSpace(raw.RetEvento.InfEvento.TpAmb)
	out.AccessKey = accessKey
	out.EventType = strings.TrimSpace(raw.RetEvento.InfEvento.TpEvento)
	out.Sequence = sequence
	out.StatusCode = itemCode
	out.Reason = strings.TrimSpace(raw.RetEvento.InfEvento.XMotivo)
	out.EventName = strings.TrimSpace(raw.RetEvento.InfEvento.XEvento)
	out.RegisteredAt = strings.TrimSpace(raw.RetEvento.InfEvento.DhRegEvento)
	out.Protocol = strings.TrimSpace(raw.RetEvento.InfEvento.NProt)
	return out, nil
}
