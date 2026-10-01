package sefaz

import (
	"context"
	"fmt"
	"strings"
)

const (
	WSDLStatusService        = SEFAZWSDLNamespacePrefix + "NFeStatusServico4"
	WSDLAuthorizationService = SEFAZWSDLNamespacePrefix + "NFeAutorizacao4"
	WSDLConsultationService  = SEFAZWSDLNamespacePrefix + "NFeConsultaProtocolo4"
)

type ServiceEndpoints struct {
	Status        Endpoint
	Authorization Endpoint
	Consultation  Endpoint
}

func (e ServiceEndpoints) Validate() error {
	for name, endpoint := range map[string]Endpoint{
		"status":        e.Status,
		"authorization": e.Authorization,
		"consultation":  e.Consultation,
	} {
		if err := endpoint.Validate(); err != nil {
			return fmt.Errorf("%s endpoint: %w", name, err)
		}
	}
	if e.Status.WSDLNamespace != WSDLStatusService {
		return fmt.Errorf("status endpoint WSDL namespace mismatch")
	}
	if e.Authorization.WSDLNamespace != WSDLAuthorizationService {
		return fmt.Errorf("authorization endpoint WSDL namespace mismatch")
	}
	if e.Consultation.WSDLNamespace != WSDLConsultationService {
		return fmt.Errorf("consultation endpoint WSDL namespace mismatch")
	}
	return nil
}

type Gateway struct {
	soap      *SOAPClient
	endpoints ServiceEndpoints
}

func NewGateway(soap *SOAPClient, endpoints ServiceEndpoints) (*Gateway, error) {
	if soap == nil {
		return nil, fmt.Errorf("SOAP client is required")
	}
	if err := endpoints.Validate(); err != nil {
		return nil, err
	}
	return &Gateway{soap: soap, endpoints: endpoints}, nil
}

func (g *Gateway) Status(ctx context.Context, env Environment, uf string) (ServiceStatusResponse, error) {
	payload, err := BuildStatusRequest(env, uf)
	if err != nil {
		return ServiceStatusResponse{}, err
	}
	response, err := g.soap.Post(ctx, g.endpoints.Status, payload)
	if err != nil {
		return ServiceStatusResponse{}, err
	}
	return ParseServiceStatusResponse(response)
}

func (g *Gateway) Authorize(
	ctx context.Context,
	idLot string,
	expectedAccessKey string,
	signedNFe []byte,
) (AuthorizationResponse, error) {
	expectedAccessKey = strings.TrimSpace(expectedAccessKey)
	if err := validateExpectedAccessKey(expectedAccessKey); err != nil {
		return AuthorizationResponse{}, err
	}
	payload, err := BuildAuthorizationBatch(idLot, signedNFe)
	if err != nil {
		return AuthorizationResponse{}, err
	}
	response, err := g.soap.Post(ctx, g.endpoints.Authorization, payload)
	if err != nil {
		return AuthorizationResponse{}, err
	}
	parsed, err := ParseAuthorizationResponse(response)
	if err != nil {
		return AuthorizationResponse{}, err
	}
	if parsed.Protocol != nil && parsed.Protocol.AccessKey != "" &&
		parsed.Protocol.AccessKey != expectedAccessKey {
		return AuthorizationResponse{}, fmt.Errorf(
			"SEFAZ authorization protocol access key mismatch: got %s want %s",
			parsed.Protocol.AccessKey, expectedAccessKey,
		)
	}
	return parsed, nil
}

func (g *Gateway) Consult(
	ctx context.Context,
	env Environment,
	accessKey string,
) (ConsultationResponse, error) {
	accessKey = strings.TrimSpace(accessKey)
	if err := validateExpectedAccessKey(accessKey); err != nil {
		return ConsultationResponse{}, err
	}
	payload, err := BuildConsultationRequest(env, accessKey)
	if err != nil {
		return ConsultationResponse{}, err
	}
	response, err := g.soap.Post(ctx, g.endpoints.Consultation, payload)
	if err != nil {
		return ConsultationResponse{}, err
	}
	parsed, err := ParseConsultationResponse(response)
	if err != nil {
		return ConsultationResponse{}, err
	}
	if parsed.AccessKey != "" && parsed.AccessKey != accessKey {
		return ConsultationResponse{}, fmt.Errorf(
			"SEFAZ consultation access key mismatch: got %s want %s",
			parsed.AccessKey, accessKey,
		)
	}
	if parsed.Protocol != nil && parsed.Protocol.AccessKey != "" &&
		parsed.Protocol.AccessKey != accessKey {
		return ConsultationResponse{}, fmt.Errorf(
			"SEFAZ consultation protocol access key mismatch: got %s want %s",
			parsed.Protocol.AccessKey, accessKey,
		)
	}
	return parsed, nil
}

func validateExpectedAccessKey(accessKey string) error {
	if err := validateAccessKey(accessKey); err != nil {
		return fmt.Errorf("expected access key: %w", err)
	}
	return nil
}
