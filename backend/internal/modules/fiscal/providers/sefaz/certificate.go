package sefaz

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type CertificateResolver interface {
	ResolveClientCertificate(ctx context.Context, secretRef string) (tls.Certificate, error)
}

func ResolveAndValidateCertificate(
	ctx context.Context,
	resolver CertificateResolver,
	secretRef string,
	now time.Time,
) (tls.Certificate, error) {
	if resolver == nil {
		return tls.Certificate{}, fmt.Errorf("certificate resolver is required")
	}
	secretRef = strings.TrimSpace(secretRef)
	if secretRef == "" {
		return tls.Certificate{}, fmt.Errorf("certificate secret reference is required")
	}
	cert, err := resolver.ResolveClientCertificate(ctx, secretRef)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("resolve client certificate: %w", err)
	}
	if err := ValidateClientCertificate(cert, now); err != nil {
		return tls.Certificate{}, err
	}
	return cert, nil
}

func ValidateClientCertificate(cert tls.Certificate, now time.Time) error {
	if cert.PrivateKey == nil {
		return fmt.Errorf("client certificate private key is missing")
	}
	if len(cert.Certificate) == 0 {
		return fmt.Errorf("client certificate chain is empty")
	}
	leaf := cert.Leaf
	if leaf == nil {
		parsed, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return fmt.Errorf("parse client certificate: %w", err)
		}
		leaf = parsed
	}
	if now.IsZero() {
		now = time.Now()
	}
	if now.Before(leaf.NotBefore) {
		return fmt.Errorf("client certificate is not valid yet")
	}
	if !now.Before(leaf.NotAfter) {
		return fmt.Errorf("client certificate is expired")
	}
	return nil
}

func NewMTLSRoundTripper(cert tls.Certificate, now time.Time) (http.RoundTripper, error) {
	if err := ValidateClientCertificate(cert, now); err != nil {
		return nil, err
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, fmt.Errorf("default HTTP transport has unexpected type")
	}
	transport := base.Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}
	return transport, nil
}
