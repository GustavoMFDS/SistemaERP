package app

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/GustavoMFDS/SistemaERP/backend/internal/config"
)

// configureRedisTLS trusts the explicitly configured, host-local certificate
// for rediss:// without changing the global operating-system trust store.
// Verification of the server hostname and certificate chain stays enabled.
func configureRedisTLS(cfg config.Config, options *redis.Options) error {
	if strings.TrimSpace(cfg.RedisCACertFile) == "" {
		return nil
	}
	if options == nil || options.TLSConfig == nil || !strings.HasPrefix(cfg.RedisURL, "rediss://") {
		return errors.New("REDIS_CA_CERT_FILE requires a rediss:// TLS URL")
	}
	host, _, err := net.SplitHostPort(options.Addr)
	if err != nil || host == "" {
		return errors.New("REDIS_CA_CERT_FILE requires a valid TLS server hostname")
	}
	data, err := os.ReadFile(cfg.RedisCACertFile)
	if err != nil {
		return fmt.Errorf("cannot load Garnet pinned TLS certificate: %w", err)
	}
	der := data
	if block, rest := pem.Decode(data); block != nil {
		if block.Type != "CERTIFICATE" || len(strings.TrimSpace(string(rest))) != 0 {
			return errors.New("invalid Garnet pinned TLS certificate")
		}
		der = block.Bytes
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return errors.New("invalid Garnet pinned TLS certificate")
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	tlsConfig := options.TLSConfig.Clone()
	tlsConfig.RootCAs = roots
	tlsConfig.ServerName = host
	if tlsConfig.MinVersion < tls.VersionTLS12 {
		tlsConfig.MinVersion = tls.VersionTLS12
	}
	if tlsConfig.InsecureSkipVerify {
		return errors.New("Garnet pinned certificate must not disable TLS verification")
	}
	options.TLSConfig = tlsConfig
	return nil
}
