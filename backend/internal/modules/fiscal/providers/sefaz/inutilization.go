package sefaz

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

const InutilizationVersion = "4.00"

type InutilizationInput struct {
	Environment   Environment
	IssuerUF      string
	IssuerCNPJ    string
	Year          int
	Series        int
	StartNumber   int64
	EndNumber     int64
	Justification string
}

type inutilizationXML struct {
	XMLName xml.Name             `xml:"inutNFe"`
	Xmlns   string               `xml:"xmlns,attr"`
	Version string               `xml:"versao,attr"`
	Info    inutilizationInfoXML `xml:"infInut"`
}

type inutilizationInfoXML struct {
	ID            string `xml:"Id,attr"`
	Environment   string `xml:"tpAmb"`
	Service       string `xml:"xServ"`
	UFCode        string `xml:"cUF"`
	Year          string `xml:"ano"`
	CNPJ          string `xml:"CNPJ"`
	Model         string `xml:"mod"`
	Series        int    `xml:"serie"`
	StartNumber   int64  `xml:"nNFIni"`
	EndNumber     int64  `xml:"nNFFin"`
	Justification string `xml:"xJust"`
}

func BuildUnsignedNFCeInutilization(input InutilizationInput) ([]byte, string, error) {
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
	if input.Year < 2006 || input.Year > 2099 {
		return nil, "", fmt.Errorf("inutilization year must be between 2006 and 2099")
	}
	if input.Series < 0 || input.Series > 889 {
		return nil, "", fmt.Errorf("inutilization series must be between 0 and 889")
	}
	if input.StartNumber < 1 || input.EndNumber > 999999999 ||
		input.StartNumber > input.EndNumber {
		return nil, "", fmt.Errorf("invalid inutilization number range")
	}
	if input.EndNumber-input.StartNumber+1 > 10000 {
		return nil, "", fmt.Errorf("inutilization range must contain at most 10000 numbers")
	}
	justification := strings.TrimSpace(input.Justification)
	if len([]rune(justification)) < 15 || len([]rune(justification)) > 255 {
		return nil, "", fmt.Errorf("inutilization justification must contain 15 to 255 characters")
	}

	cnpj := normalizedCNPJ(input.IssuerCNPJ)
	year := fmt.Sprintf("%02d", input.Year%100)
	requestID := fmt.Sprintf(
		"ID%s%s%s65%03d%09d%09d",
		cUF,
		year,
		cnpj,
		input.Series,
		input.StartNumber,
		input.EndNumber,
	)
	if len(requestID) != 43 {
		return nil, "", fmt.Errorf("invalid inutilization request Id length")
	}

	doc := inutilizationXML{
		Xmlns:   NFeNamespace,
		Version: InutilizationVersion,
		Info: inutilizationInfoXML{
			ID:            requestID,
			Environment:   ambient,
			Service:       "INUTILIZAR",
			UFCode:        cUF,
			Year:          year,
			CNPJ:          cnpj,
			Model:         "65",
			Series:        input.Series,
			StartNumber:   input.StartNumber,
			EndNumber:     input.EndNumber,
			Justification: justification,
		},
	}
	payload, err := xml.Marshal(doc)
	if err != nil {
		return nil, "", err
	}
	return append([]byte(xml.Header), payload...), requestID, nil
}

type InutilizationResponse struct {
	Environment string
	StatusCode  int
	Reason      string
	UFCode      string
	Year        string
	CNPJ        string
	Model       string
	Series      int
	StartNumber int64
	EndNumber   int64
	ReceivedAt  string
	Protocol    string
}

type retInutilizationXML struct {
	Info struct {
		Environment string `xml:"tpAmb"`
		StatusCode  string `xml:"cStat"`
		Reason      string `xml:"xMotivo"`
		UFCode      string `xml:"cUF"`
		Year        string `xml:"ano"`
		CNPJ        string `xml:"CNPJ"`
		Model       string `xml:"mod"`
		Series      string `xml:"serie"`
		StartNumber string `xml:"nNFIni"`
		EndNumber   string `xml:"nNFFin"`
		ReceivedAt  string `xml:"dhRecbto"`
		Protocol    string `xml:"nProt"`
	} `xml:"infInut"`
}

func ParseInutilizationResponse(content []byte) (InutilizationResponse, error) {
	var raw retInutilizationXML
	if err := decodeElementByLocalName(content, "retInutNFe", &raw); err != nil {
		return InutilizationResponse{}, err
	}
	code, err := parseStatusCode(raw.Info.StatusCode)
	if err != nil {
		return InutilizationResponse{}, fmt.Errorf("inutilization cStat: %w", err)
	}
	out := InutilizationResponse{
		Environment: strings.TrimSpace(raw.Info.Environment),
		StatusCode:  code,
		Reason:      strings.TrimSpace(raw.Info.Reason),
		UFCode:      strings.TrimSpace(raw.Info.UFCode),
		Year:        strings.TrimSpace(raw.Info.Year),
		CNPJ:        strings.TrimSpace(raw.Info.CNPJ),
		Model:       strings.TrimSpace(raw.Info.Model),
		ReceivedAt:  strings.TrimSpace(raw.Info.ReceivedAt),
		Protocol:    strings.TrimSpace(raw.Info.Protocol),
	}
	if v := strings.TrimSpace(raw.Info.Series); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return InutilizationResponse{}, fmt.Errorf("inutilization series: %w", err)
		}
		out.Series = parsed
	}
	if v := strings.TrimSpace(raw.Info.StartNumber); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return InutilizationResponse{}, fmt.Errorf("inutilization start number: %w", err)
		}
		out.StartNumber = parsed
	}
	if v := strings.TrimSpace(raw.Info.EndNumber); v != "" {
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return InutilizationResponse{}, fmt.Errorf("inutilization end number: %w", err)
		}
		out.EndNumber = parsed
	}
	return out, nil
}

func inutilizationRemoteResult(
	requestID string,
	expected InutilizationInput,
	response InutilizationResponse,
	responseXML []byte,
) (fisc.NFCeInutilizationRemoteResult, error) {
	out := fisc.NFCeInutilizationRemoteResult{
		RequestID:   requestID,
		StatusCode:  response.StatusCode,
		Reason:      response.Reason,
		ResponseXML: append([]byte(nil), responseXML...),
	}
	if response.StatusCode != 102 {
		out.FinalStatus = fisc.NFCeInutilizationStatusRejected
		return out, nil
	}
	if response.Protocol == "" || response.ReceivedAt == "" {
		return fisc.NFCeInutilizationRemoteResult{}, fmt.Errorf("registered inutilization response is incomplete")
	}
	if response.Model != "65" ||
		response.Series != expected.Series ||
		response.StartNumber != expected.StartNumber ||
		response.EndNumber != expected.EndNumber ||
		response.Year != fmt.Sprintf("%02d", expected.Year%100) ||
		normalizedCNPJ(response.CNPJ) != normalizedCNPJ(expected.IssuerCNPJ) {
		return fisc.NFCeInutilizationRemoteResult{}, fmt.Errorf("inutilization response identity mismatch")
	}
	registeredAt, err := time.Parse(time.RFC3339, response.ReceivedAt)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, fmt.Errorf("parse inutilization receipt timestamp: %w", err)
	}
	out.FinalStatus = fisc.NFCeInutilizationStatusRegistered
	out.Protocol = response.Protocol
	out.RegisteredAt = registeredAt
	return out, nil
}
