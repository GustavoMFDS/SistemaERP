package lanrotation

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	newKey := func() *ecdsa.PrivateKey {
		t.Helper()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		return key
	}
	create := func(tmpl, parent *x509.Certificate, pub, signer *ecdsa.PrivateKey) []byte {
		t.Helper()
		der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &pub.PublicKey, signer)
		if err != nil {
			t.Fatal(err)
		}
		return der
	}
	rootKey := newKey()
	root := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic root"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(48 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER := create(root, root, rootKey, rootKey)
	parsedRoot, err := x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsedRoot)
	key := newKey()
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), DNSNames: []string{"erp.shop.test"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der := create(leaf, parsedRoot, key, rootKey)
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := func(k *ecdsa.PrivateKey) []byte {
		t.Helper()
		b, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b})
	}
	priv := keyPEM(key)
	hash := sha256.Sum256(der)
	pin := hex.EncodeToString(hash[:])
	const origin = "https://erp.shop.test:8443"
	cases := []struct {
		name, origin, pin string
		port              int
		cert, key         []byte
		roots             *x509.CertPool
		at                time.Time
		ok                bool
	}{
		{"valid", origin, pin, 8443, cert, priv, roots, now, true},
		{"wrong-host", "https://other.shop.test:8443", pin, 8443, cert, priv, roots, now, false},
		{"ip", "https://192.168.0.102:8443", pin, 8443, cert, priv, roots, now, false},
		{"http", "http://erp.shop.test:8443", pin, 8443, cert, priv, roots, now, false},
		{"path", origin+"/login", pin, 8443, cert, priv, roots, now, false},
		{"query", origin+"?x=1", pin, 8443, cert, priv, roots, now, false},
		{"fragment", origin+"#", pin, 8443, cert, priv, roots, now, false},
		{"userinfo", "https://user@erp.shop.test:8443", pin, 8443, cert, priv, roots, now, false},
		{"port", origin, pin, 443, cert, priv, roots, now, false},
		{"expired", origin, pin, 8443, cert, priv, roots, now.Add(2*time.Hour), false},
		{"not-yet-valid", origin, pin, 8443, cert, priv, roots, now.Add(-2*time.Hour), false},
		{"untrusted", origin, pin, 8443, cert, priv, x509.NewCertPool(), now, false},
		{"missing-roots", origin, pin, 8443, cert, priv, nil, now, false},
		{"wrong-key", origin, pin, 8443, cert, keyPEM(newKey()), roots, now, false},
		{"wrong-pin", origin, hex.EncodeToString(make([]byte, 32)), 8443, cert, priv, roots, now, false},
		{"invalid-pem", origin, pin, 8443, []byte("invalid"), priv, roots, now, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.origin, tc.port, tc.cert, tc.key, tc.pin, tc.roots, tc.at)
			if (err == nil) != tc.ok {
				t.Fatalf("expected success=%v, got %v", tc.ok, err)
			}
		})
	}
}
