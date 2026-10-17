package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Resolve the listener's TLS mode before accepting any network connections.
// A locally bundled SPA cannot be served over plaintext on a LAN because
// browser credentials and PWA features require a trusted secure origin.
func resolveLocalTLS(webDir, address, certFile, keyFile string) (bool, error) {
	certFile = strings.TrimSpace(certFile)
	keyFile = strings.TrimSpace(keyFile)
	if (certFile == "") != (keyFile == "") {
		return false, errors.New("both ESTERIUM_TLS_CERT_FILE and ESTERIUM_TLS_KEY_FILE are required")
	}
	if certFile == "" {
		if strings.TrimSpace(webDir) != "" && !isLoopbackBind(address) {
			return false, errors.New("local frontend on a LAN requires HTTPS; bind to loopback or configure a trusted TLS certificate")
		}
		return false, nil
	}

	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return false, fmt.Errorf("local HTTPS certificate/key invalid: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return false, errors.New("local HTTPS certificate is empty")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return false, fmt.Errorf("parse local HTTPS certificate: %w", err)
	}
	now := time.Now()
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return false, errors.New("local HTTPS certificate is not currently valid")
	}
	return true, nil
}

func isLoopbackBind(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
