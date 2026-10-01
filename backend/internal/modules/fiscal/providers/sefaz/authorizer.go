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
