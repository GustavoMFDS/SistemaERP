package sefaz

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

type SEFAZAuthorizer struct {
	resolver    CertificateResolver
	timeout     time.Duration
	environment Environment
}

func NewHomologationAuthorizer(resolver CertificateResolver, timeout time.Duration) *SEFAZAuthorizer {
	return newSEFAZAuthorizer(resolver, timeout, EnvironmentHomologation)
}

func NewProductionAuthorizer(resolver CertificateResolver, timeout time.Duration) *SEFAZAuthorizer {
	return newSEFAZAuthorizer(resolver, timeout, EnvironmentProduction)
}

func newSEFAZAuthorizer(
	resolver CertificateResolver,
	timeout time.Duration,
	environment Environment,
) *SEFAZAuthorizer {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &SEFAZAuthorizer{resolver: resolver, timeout: timeout, environment: environment}
}

func (a *SEFAZAuthorizer) validateEnvironment(environment string) error {
	if a == nil {
		return fmt.Errorf("SEFAZ authorizer is not configured")
	}
	if Environment(strings.TrimSpace(environment)) != a.environment {
		return fmt.Errorf("SEFAZ authorizer is restricted to %s", a.environment)
	}
	return nil
}

func (a *SEFAZAuthorizer) Authorize(
	ctx context.Context,
	certificateSecretRef string,
	issuerUF string,
	environment string,
	accessKey string,
	documentNumber int64,
	signedXML []byte,
) (fisc.NFCeRemoteOutcome, error) {
	if err := a.validateEnvironment(environment); err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	if documentNumber < 1 || documentNumber > 999999999 {
		return fisc.NFCeRemoteOutcome{}, fmt.Errorf("invalid NFC-e document number")
	}
	gateway, err := a.gateway(ctx, certificateSecretRef, issuerUF)
	if err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	response, err := gateway.Authorize(
		ctx,
		strconv.FormatInt(documentNumber, 10),
		accessKey,
		signedXML,
	)
	if err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	return authorizationOutcome(accessKey, response)
}

func (a *SEFAZAuthorizer) Consult(
	ctx context.Context,
	certificateSecretRef string,
	issuerUF string,
	environment string,
	accessKey string,
) (fisc.NFCeRemoteOutcome, error) {
	if err := a.validateEnvironment(environment); err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	gateway, err := a.gateway(ctx, certificateSecretRef, issuerUF)
	if err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	response, err := gateway.Consult(ctx, a.environment, accessKey)
	if err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	return consultationOutcome(accessKey, response)
}

func (a *SEFAZAuthorizer) gateway(
	ctx context.Context,
	certificateSecretRef string,
	issuerUF string,
) (*Gateway, error) {
	if a == nil || a.resolver == nil {
		return nil, fmt.Errorf("SEFAZ authorizer is not configured")
	}
	cert, err := ResolveAndValidateCertificate(
		ctx,
		a.resolver,
		certificateSecretRef,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	transport, err := NewMTLSRoundTripper(cert, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	entry, err := ResolveCatalogEntry(issuerUF, a.environment)
	if err != nil {
		return nil, err
	}
	return NewGateway(NewSOAPClient(transport, a.timeout), entry.Services)
}

func authorizationOutcome(
	expectedAccessKey string,
	response AuthorizationResponse,
) (fisc.NFCeRemoteOutcome, error) {
	out := fisc.NFCeRemoteOutcome{
		AccessKey:  expectedAccessKey,
		StatusCode: response.StatusCode,
		Reason:     strings.TrimSpace(response.Reason),
	}
	if response.Protocol == nil {
		return out, nil
	}
	out.StatusCode = response.Protocol.StatusCode
	out.Reason = strings.TrimSpace(response.Protocol.Reason)
	out.Protocol = strings.TrimSpace(response.Protocol.Protocol)
	if response.Protocol.AccessKey != "" && response.Protocol.AccessKey != expectedAccessKey {
		return fisc.NFCeRemoteOutcome{}, fmt.Errorf("authorization outcome access key mismatch")
	}
	if response.Protocol.StatusCode == 100 {
		receivedAt, err := parseProtocolTime(response.Protocol.ReceivedAt)
		if err != nil {
			return fisc.NFCeRemoteOutcome{}, err
		}
		out.FinalStatus = fisc.NFCeStatusAuthorized
		out.ReceivedAt = receivedAt
		return out, nil
	}
	out.FinalStatus = fisc.NFCeStatusRejected
	return out, nil
}

func consultationOutcome(
	expectedAccessKey string,
	response ConsultationResponse,
) (fisc.NFCeRemoteOutcome, error) {
	out := fisc.NFCeRemoteOutcome{
		AccessKey:  expectedAccessKey,
		StatusCode: response.StatusCode,
		Reason:     strings.TrimSpace(response.Reason),
	}
	if response.AccessKey != "" && response.AccessKey != expectedAccessKey {
		return fisc.NFCeRemoteOutcome{}, fmt.Errorf("consultation outcome access key mismatch")
	}
	if response.Protocol == nil {
		return out, nil
	}
	if response.Protocol.AccessKey != "" && response.Protocol.AccessKey != expectedAccessKey {
		return fisc.NFCeRemoteOutcome{}, fmt.Errorf("consultation protocol access key mismatch")
	}
	out.StatusCode = response.Protocol.StatusCode
	out.Reason = strings.TrimSpace(response.Protocol.Reason)
	out.Protocol = strings.TrimSpace(response.Protocol.Protocol)
	if response.Protocol.StatusCode == 100 {
		receivedAt, err := parseProtocolTime(response.Protocol.ReceivedAt)
		if err != nil {
			return fisc.NFCeRemoteOutcome{}, err
		}
		out.FinalStatus = fisc.NFCeStatusAuthorized
		out.ReceivedAt = receivedAt
		return out, nil
	}
	out.FinalStatus = fisc.NFCeStatusRejected
	return out, nil
}

func parseProtocolTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("SEFAZ protocol receipt timestamp is missing")
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse SEFAZ protocol timestamp: %w", err)
	}
	return parsed, nil
}

func (a *SEFAZAuthorizer) Cancel(
	ctx context.Context,
	certificateSecretRef string,
	issuerUF string,
	environment string,
	accessKey string,
	documentNumber int64,
	eventID string,
	sequence int,
	signedEventXML []byte,
) (fisc.NFCeCancellationRemoteResult, error) {
	if err := a.validateEnvironment(environment); err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	if err := validateExpectedAccessKey(strings.TrimSpace(accessKey)); err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	if documentNumber < 1 || documentNumber > 999999999 {
		return fisc.NFCeCancellationRemoteResult{}, fmt.Errorf("invalid NFC-e document number")
	}
	if sequence < 1 || sequence > 99 {
		return fisc.NFCeCancellationRemoteResult{}, fmt.Errorf("invalid cancellation event sequence")
	}
	wantEventID := fmt.Sprintf("ID%s%s%02d", CancellationEventType, strings.TrimSpace(accessKey), sequence)
	if strings.TrimSpace(eventID) != wantEventID {
		return fisc.NFCeCancellationRemoteResult{}, fmt.Errorf("cancellation event Id mismatch")
	}

	if a == nil || a.resolver == nil {
		return fisc.NFCeCancellationRemoteResult{}, fmt.Errorf("SEFAZ authorizer is not configured")
	}
	cert, err := ResolveAndValidateCertificate(
		ctx,
		a.resolver,
		certificateSecretRef,
		time.Now().UTC(),
	)
	if err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	transport, err := NewMTLSRoundTripper(cert, time.Now().UTC())
	if err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	entry, err := ResolveCatalogEntry(issuerUF, a.environment)
	if err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	payload, err := BuildCancellationEventBatch(
		strconv.FormatInt(documentNumber, 10),
		signedEventXML,
	)
	if err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	endpoint := Endpoint{
		URL:           entry.EventURL,
		WSDLNamespace: SEFAZWSDLNamespacePrefix + "NFeRecepcaoEvento4",
	}
	responseXML, err := NewSOAPClient(transport, a.timeout).Post(ctx, endpoint, payload)
	if err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}
	response, err := ParseEventResponse(responseXML)
	if err != nil {
		return fisc.NFCeCancellationRemoteResult{}, err
	}

	out := fisc.NFCeCancellationRemoteResult{
		AccessKey:   strings.TrimSpace(accessKey),
		EventID:     wantEventID,
		Sequence:    sequence,
		StatusCode:  response.BatchStatusCode,
		Reason:      response.BatchReason,
		ResponseXML: append([]byte(nil), responseXML...),
	}
	if response.EventType == "" {
		return out, nil
	}
	if response.AccessKey != accessKey {
		return fisc.NFCeCancellationRemoteResult{}, fmt.Errorf("cancellation response access key mismatch")
	}
	if response.EventType != CancellationEventType || response.Sequence != sequence {
		return fisc.NFCeCancellationRemoteResult{}, fmt.Errorf("cancellation response event identity mismatch")
	}
	out.StatusCode = response.StatusCode
	out.Reason = response.Reason
	out.Protocol = response.Protocol
	if response.CancellationRegistered() {
		registeredAt, err := parseProtocolTime(response.RegisteredAt)
		if err != nil {
			return fisc.NFCeCancellationRemoteResult{}, err
		}
		out.FinalStatus = fisc.NFCeEventStatusRegistered
		out.RegisteredAt = registeredAt
		return out, nil
	}
	out.FinalStatus = fisc.NFCeEventStatusRejected
	return out, nil
}

func (a *SEFAZAuthorizer) Inutilize(
	ctx context.Context,
	certificateSecretRef string,
	draft fisc.NFCeInutilizationDraft,
	requestID string,
	signedXML []byte,
) (fisc.NFCeInutilizationRemoteResult, error) {
	if err := a.validateEnvironment(draft.Environment); err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	requestID = strings.TrimSpace(requestID)
	if len(requestID) != 43 || !strings.HasPrefix(requestID, "ID") {
		return fisc.NFCeInutilizationRemoteResult{}, fmt.Errorf("invalid inutilization request Id")
	}
	if a == nil || a.resolver == nil {
		return fisc.NFCeInutilizationRemoteResult{}, fmt.Errorf("SEFAZ authorizer is not configured")
	}
	cert, err := ResolveAndValidateCertificate(
		ctx,
		a.resolver,
		certificateSecretRef,
		time.Now().UTC(),
	)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	transport, err := NewMTLSRoundTripper(cert, time.Now().UTC())
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	entry, err := ResolveCatalogEntry(draft.IssuerUF, a.environment)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	endpoint := Endpoint{
		URL:           entry.InutilizationURL,
		WSDLNamespace: SEFAZWSDLNamespacePrefix + "NFeInutilizacao4",
	}
	responseXML, err := NewSOAPClient(transport, a.timeout).Post(
		ctx,
		endpoint,
		signedXML,
	)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	response, err := ParseInutilizationResponse(responseXML)
	if err != nil {
		return fisc.NFCeInutilizationRemoteResult{}, err
	}
	ambient, _ := tpAmb(a.environment)
	if response.Environment != "" && response.Environment != ambient {
		return fisc.NFCeInutilizationRemoteResult{}, fmt.Errorf("inutilization response environment mismatch")
	}
	return inutilizationRemoteResult(
		requestID,
		InutilizationInput{
			Environment:   a.environment,
			IssuerUF:      draft.IssuerUF,
			IssuerCNPJ:    draft.IssuerCNPJ,
			Year:          draft.Year,
			Series:        draft.Series,
			StartNumber:   draft.StartNumber,
			EndNumber:     draft.EndNumber,
			Justification: draft.Justification,
		},
		response,
		responseXML,
	)
}
