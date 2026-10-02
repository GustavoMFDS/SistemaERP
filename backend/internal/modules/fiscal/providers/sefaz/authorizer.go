package sefaz

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	fisc "github.com/example/sistemaemgo/internal/modules/fiscal/domain"
)

type HomologationAuthorizer struct {
	resolver CertificateResolver
	timeout  time.Duration
}

func NewHomologationAuthorizer(resolver CertificateResolver, timeout time.Duration) *HomologationAuthorizer {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &HomologationAuthorizer{resolver: resolver, timeout: timeout}
}

func (a *HomologationAuthorizer) Authorize(
	ctx context.Context,
	certificateSecretRef string,
	issuerUF string,
	environment string,
	accessKey string,
	documentNumber int64,
	signedXML []byte,
) (fisc.NFCeRemoteOutcome, error) {
	if strings.TrimSpace(environment) != string(EnvironmentHomologation) {
		return fisc.NFCeRemoteOutcome{}, fmt.Errorf("SEFAZ authorizer is restricted to homologation")
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

func (a *HomologationAuthorizer) Consult(
	ctx context.Context,
	certificateSecretRef string,
	issuerUF string,
	environment string,
	accessKey string,
) (fisc.NFCeRemoteOutcome, error) {
	if strings.TrimSpace(environment) != string(EnvironmentHomologation) {
		return fisc.NFCeRemoteOutcome{}, fmt.Errorf("SEFAZ authorizer is restricted to homologation")
	}
	gateway, err := a.gateway(ctx, certificateSecretRef, issuerUF)
	if err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	response, err := gateway.Consult(ctx, EnvironmentHomologation, accessKey)
	if err != nil {
		return fisc.NFCeRemoteOutcome{}, err
	}
	return consultationOutcome(accessKey, response)
}

func (a *HomologationAuthorizer) gateway(
	ctx context.Context,
	certificateSecretRef string,
	issuerUF string,
) (*Gateway, error) {
	if a == nil || a.resolver == nil {
		return nil, fmt.Errorf("homologation authorizer is not configured")
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
	entry, err := ResolveCatalogEntry(issuerUF, EnvironmentHomologation)
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


func (a *HomologationAuthorizer) Cancel(
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
	if strings.TrimSpace(environment) != string(EnvironmentHomologation) {
		return fisc.NFCeCancellationRemoteResult{}, fmt.Errorf("SEFAZ cancellation is restricted to homologation")
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
		return fisc.NFCeCancellationRemoteResult{}, fmt.Errorf("homologation authorizer is not configured")
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
	entry, err := ResolveCatalogEntry(issuerUF, EnvironmentHomologation)
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
		AccessKey:  strings.TrimSpace(accessKey),
		EventID:    wantEventID,
		Sequence:   sequence,
		StatusCode: response.BatchStatusCode,
		Reason:     response.BatchReason,
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
