package sefaz

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/http"
	"testing"
	"time"
)

type staticCertificateResolver struct {
	cert tls.Certificate
	err  error
	ref  string
}

func (r *staticCertificateResolver) ResolveClientCertificate(_ context.Context, secretRef string) (tls.Certificate, error) {
	r.ref = secretRef
	return r.cert, r.err
}

func testCertificate(t *testing.T, notBefore, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "SistemaEmGo NFC-e test"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        leaf,
	}
}

func TestResolveAndValidateCertificate(t *testing.T) {
	now := time.Date(2026, time.September, 30, 23, 0, 0, 0, time.UTC)
	resolver := &staticCertificateResolver{
		cert: testCertificate(t, now.Add(-time.Hour), now.Add(24*time.Hour)),
	}
	cert, err := ResolveAndValidateCertificate(context.Background(), resolver, "secret://nfce/a1", now)
	if err != nil {
		t.Fatalf("ResolveAndValidateCertificate: %v", err)
	}
	if resolver.ref != "secret://nfce/a1" || cert.PrivateKey == nil {
		t.Fatalf("unexpected resolved certificate")
	}
}

func TestValidateClientCertificateRejectsExpiredFutureAndMissingKey(t *testing.T) {
	now := time.Date(2026, time.September, 30, 23, 0, 0, 0, time.UTC)

	expired := testCertificate(t, now.Add(-48*time.Hour), now.Add(-time.Hour))
	if err := ValidateClientCertificate(expired, now); err == nil {
		t.Fatal("expected expired certificate to fail")
	}

	future := testCertificate(t, now.Add(time.Hour), now.Add(48*time.Hour))
	if err := ValidateClientCertificate(future, now); err == nil {
		t.Fatal("expected future certificate to fail")
	}

	missingKey := testCertificate(t, now.Add(-time.Hour), now.Add(time.Hour))
	missingKey.PrivateKey = nil
	if err := ValidateClientCertificate(missingKey, now); err == nil {
		t.Fatal("expected missing private key to fail")
	}
}

func TestNewMTLSRoundTripper(t *testing.T) {
	now := time.Date(2026, time.September, 30, 23, 0, 0, 0, time.UTC)
	cert := testCertificate(t, now.Add(-time.Hour), now.Add(24*time.Hour))
	rt, err := NewMTLSRoundTripper(cert, now)
	if err != nil {
		t.Fatalf("NewMTLSRoundTripper: %v", err)
	}
	transport, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("round tripper type=%T", rt)
	}
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TLS minimum not enforced")
	}
	if len(transport.TLSClientConfig.Certificates) != 1 {
		t.Fatalf("certificate count=%d, want 1", len(transport.TLSClientConfig.Certificates))
	}
}
