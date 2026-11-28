// Package lanrotation provides a pure, read-only preflight for a proposed LAN
// TLS identity. It does not configure SCM, DNS, the firewall or trust stores.
package lanrotation

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Validate checks a canonical HTTPS DNS origin, its listener port, a PEM
// keypair, an independently approved leaf pin and a chain rooted in the
// caller's independently approved trust pool. The caller owns the input
// buffers and must never log key material. No revocation or live reachability
// claim is made; the existing terminal acceptance probe remains mandatory.
func Validate(origin string, port int, certPEM, keyPEM []byte, expectedSHA256 string, roots *x509.CertPool, now time.Time) error {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" ||
		u.RawQuery != "" || u.ForceQuery || strings.Contains(origin, "#") ||
		(u.Path != "" && u.Path != "/") || u.RawPath != "" || port < 1 || port > 65535 {
		return errors.New("invalid HTTPS origin or listener port")
	}
	host := u.Hostname()
	if !validDNSName(host) {
		return errors.New("stable DNS hostname required")
	}
	want := "https://" + host
	if port != 443 {
		want += ":" + strconv.Itoa(port)
	}
	if origin != want && origin != want+"/" {
		return errors.New("origin must be canonical and match listener port")
	}
	pin, err := hex.DecodeString(expectedSHA256)
	if err != nil || len(pin) != sha256.Size || roots == nil || now.IsZero() {
		return errors.New("approved leaf pin, trust roots and evaluation time required")
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil || len(pair.Certificate) == 0 {
		return errors.New("invalid TLS certificate/key pair")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || leaf.IsCA {
		return errors.New("invalid TLS leaf certificate")
	}
	hash := sha256.Sum256(leaf.Raw)
	if subtle.ConstantTimeCompare(hash[:], pin) != 1 {
		return errors.New("TLS leaf pin mismatch")
	}
	intermediates := x509.NewCertPool()
	for _, der := range pair.Certificate[1:] {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return errors.New("invalid TLS intermediate")
		}
		intermediates.AddCert(cert)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		DNSName: host, Roots: roots, Intermediates: intermediates,
		CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		return errors.New("TLS hostname, validity, usage or chain verification failed")
	}
	return nil
}

func validDNSName(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") ||
		host != strings.ToLower(host) || net.ParseIP(host) != nil ||
		host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
