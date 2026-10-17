package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveLocalTLSRejectsPlaintextLAN(t *testing.T) {
	cases := []struct {
		name    string
		web     string
		addr    string
		wantErr bool
	}{
		{"API only preserves reverse proxy compatibility", "", ":8080", false},
		{"local UI loopback", "web", "127.0.0.1:8080", false},
		{"local UI IPv6 loopback", "web", "[::1]:8080", false},
		{"local UI localhost", "web", "localhost:8080", false},
		{"all interfaces", "web", ":8080", true},
		{"IPv4 LAN", "web", "192.168.0.10:8080", true},
		{"IPv4 wildcard", "web", "0.0.0.0:8080", true},
		{"IPv6 wildcard", "web", "[::]:8080", true},
		{"malformed listener", "web", "localhost", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tlsEnabled, err := resolveLocalTLS(tc.web, tc.addr, "", "")
			if tlsEnabled {
				t.Fatal("unexpected TLS without certificate files")
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v, want error=%v", err, tc.wantErr)
			}
		})
	}
}

func TestResolveLocalTLSRequiresCompleteValidKeyPair(t *testing.T) {
	if _, err := resolveLocalTLS("web", ":8080", "cert.pem", ""); err == nil {
		t.Fatal("accepted certificate without private key")
	}
	if _, err := resolveLocalTLS("web", ":8080", "", "key.pem"); err == nil {
		t.Fatal("accepted private key without certificate")
	}

	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	writeKeyPair := func(name string, notBefore, notAfter time.Time) (string, string) {
		t.Helper()
		template := &x509.Certificate{
			SerialNumber: big.NewInt(1234),
			NotBefore:    notBefore,
			NotAfter:     notAfter,
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			DNSNames:     []string{"localhost"},
			IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		}
		der, certErr := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
		if certErr != nil {
			t.Fatal(certErr)
		}
		keyDER, keyErr := x509.MarshalECPrivateKey(key)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		certPath := filepath.Join(dir, name+"-cert.pem")
		keyPath := filepath.Join(dir, name+"-key.pem")
		if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0600); err != nil {
			t.Fatal(err)
		}
		return certPath, keyPath
	}

	now := time.Now()
	certPath, keyPath := writeKeyPair("valid", now.Add(-time.Hour), now.Add(time.Hour))
	enabled, err := resolveLocalTLS("web", ":8080", certPath, keyPath)
	if err != nil || !enabled {
		t.Fatalf("valid TLS pair rejected: enabled=%v err=%v", enabled, err)
	}
	_, err = resolveLocalTLS("web", ":8080", certPath, filepath.Join(dir, "missing.pem"))
	if err == nil {
		t.Fatal("accepted missing private key")
	}
	expiredCert, expiredKey := writeKeyPair("expired", now.Add(-3*time.Hour), now.Add(-time.Hour))
	if _, err := resolveLocalTLS("web", ":8080", expiredCert, expiredKey); err == nil {
		t.Fatal("accepted expired TLS certificate")
	}
}
